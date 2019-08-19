//
// Copyright (c) 2017
// Cavium
// Mainflux
// IOTech
// Copyright (c) 2018 Dell Technologies, Inc.
// Copyright (c) 2019 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package distro

// TODO:
// - Event buffer management per sender(do not block distro.Loop on full
//   registration channel)

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/clients"
	contract "github.com/edgexfoundry/go-mod-core-contracts/models"
	msgTypes "github.com/edgexfoundry/go-mod-messaging/pkg/types"

	"github.com/pkg/errors"
)

const (
	awsMQTTPort         int    = 8883
	awsThingUpdateTopic string = "$aws/things/%s/shadow/update"
)

var registrationChanges chan contract.NotifyUpdate = make(chan contract.NotifyUpdate, 2)

// RegistrationInfo - registration info
type registrationInfo struct {
	registration contract.Registration
	format       formatter
	compression  transformer
	encrypt      transformer
	sender       sender
	filter       []filterer

	chRegistration chan *contract.Registration
	chMessages     chan msgTypes.MessageEnvelope

	deleteFlag bool
}

func RefreshRegistrations(update contract.NotifyUpdate) {
	// TODO make it not blocking, return bool?
	registrationChanges <- update
}

func newRegistrationInfo() *registrationInfo {
	reg := &registrationInfo{}

	reg.chRegistration = make(chan *contract.Registration)
	reg.chMessages = make(chan msgTypes.MessageEnvelope)
	return reg
}

func (reg *registrationInfo) update(newReg contract.Registration) bool {
	reg.registration = newReg

	reg.format = nil
	switch newReg.Format {
	case contract.FormatJSON:
		reg.format = jsonFormatter{}
	case contract.FormatXML:
		reg.format = xmlFormatter{}
	case contract.FormatSerialized:
		reg.format = jsonFormatter{}
	case contract.FormatIoTCoreJSON:
		reg.format = jsonFormatter{}
	case contract.FormatAzureJSON:
		reg.format = azureFormatter{}
	case contract.FormatAWSJSON:
		reg.format = awsFormatter{}
	case contract.FormatCSV:
		// TODO reg.format = distro.NewCsvFormat()
	case contract.FormatThingsBoardJSON:
		reg.format = thingsboardJSONFormatter{}
	case contract.FormatNOOP:
		reg.format = noopFormatter{}
	default:
		LoggingClient.Warn(fmt.Sprintf("Format not supported: %s", newReg.Format))
		return false
	}

	reg.compression = nil
	switch newReg.Compression {
	case "":
		fallthrough
	case contract.CompNone:
		reg.compression = nil
	case contract.CompGzip:
		reg.compression = &gzipTransformer{}
	case contract.CompZip:
		reg.compression = &zlibTransformer{}
	default:
		LoggingClient.Warn(fmt.Sprintf("Compression not supported: %s", newReg.Compression))
		return false
	}

	reg.sender = nil
	switch newReg.Destination {
	case contract.DestMQTT, contract.DestAzureMQTT:
		c := Configuration.Certificates["MQTTS"]
		reg.sender = newMqttSender(newReg.Addressable, c.Cert, c.Key)
	case contract.DestAWSMQTT:
		newReg.Addressable.Protocol = "tls"
		newReg.Addressable.Path = ""
		newReg.Addressable.Topic = fmt.Sprintf(awsThingUpdateTopic, newReg.Addressable.Topic)
		newReg.Addressable.Port = awsMQTTPort
		c := Configuration.Certificates["AWS"]
		reg.sender = newMqttSender(newReg.Addressable, c.Cert, c.Key)
	case contract.DestZMQ:
		reg.sender = newZeroMQEventPublisher()
	case contract.DestIotCoreMQTT:
		reg.sender = newIoTCoreSender(newReg.Addressable)
	case contract.DestRest:
		reg.sender = newHTTPSender(newReg.Addressable)
	case contract.DestXMPP:
		reg.sender = newXMPPSender(newReg.Addressable)

	default:
		LoggingClient.Warn(fmt.Sprintf("Destination not supported: %s", newReg.Destination))
		return false
	}

	if reg.sender == nil {
		return false
	}

	reg.encrypt = nil
	switch newReg.Encryption.Algo {
	case "":
		fallthrough
	case contract.EncNone:
		reg.encrypt = nil
	case contract.EncAes:
		reg.encrypt = newAESEncryption(newReg.Encryption)
	default:
		LoggingClient.Warn(fmt.Sprintf("Encryption not supported: %s", newReg.Encryption.Algo))
		return false
	}

	reg.filter = nil

	if len(newReg.Filter.DeviceIDs) > 0 {
		reg.filter = append(reg.filter, newDevIdFilter(newReg.Filter))
		LoggingClient.Debug(fmt.Sprintf("Device ID filter added: %s", newReg.Filter.DeviceIDs))
	}

	if len(newReg.Filter.ValueDescriptorIDs) > 0 {
		reg.filter = append(reg.filter, newValueDescFilter(newReg.Filter))
		LoggingClient.Debug(fmt.Sprintf("Value descriptor filter added: %s", newReg.Filter.ValueDescriptorIDs))
	}

	return true
}

func (reg registrationInfo) processMessage(msg msgTypes.MessageEnvelope) {
	var err error
	ctx := context.WithValue(context.Background(), clients.CorrelationHeader, msg.CorrelationID)
	ctx = context.WithValue(ctx, clients.ContentType, msg.ContentType)
	switch msg.ContentType {
	case clients.ContentTypeJSON:
		err = reg.handleJSON(msg, ctx)
	case clients.ContentTypeCBOR:
		err = reg.handleCBOR(msg, ctx)
	default:
		err = errors.Errorf("unsupported %s provided: %s", clients.ContentType, msg.ContentType)
	}

	if err != nil {
		LoggingClient.Error(err.Error())
	}

	LoggingClient.Debug(fmt.Sprintf("Sent event with registration: %s", reg.registration.Name))
}

func (reg registrationInfo) handleJSON(msg msgTypes.MessageEnvelope, ctx context.Context) (err error) {
	str := string(msg.Payload)
	event := parseEvent(str)
	if event == nil {
		return errors.New("unable to parse event from string " + str)
	}

	LoggingClient.Debug(fmt.Sprintf("%s Processing Event event: %s",
		event.Device, event.ID.Hex()))

	data := event.ToContract()

	for _, f := range reg.filter {
		var accepted bool
		accepted, data = f.Filter(data)
		if !accepted {
			LoggingClient.Debug(fmt.Sprintf("%s Event filtered: %s", event.Device, event.ID.Hex()))
			return
		}
	}

	// Due to rate limits, we might see an event on the ZeroMQ
	// bus which we might have already send
	// consider the case where edge goes offline for 24 hrs.
	// It would have accumulated 1440 events (60 * 24) in that time period
	// On coming back online it would have taken approximately 24 minutes to
	// clear the backlog and during this timeframe core-data would have
	// resubmitted the same event multiple times for retransmit
	// This approach of first checking if event is still in db and not yet
	// marked as pushed avoids resending the same event multiple times due
	// to the race condiotion describe above
	evtdb, err := ec.Event(event.ID.Hex())
	if err != nil {
		LoggingClient.Error(fmt.Sprintf("%s Ignored event %s, removed from DB",
			event.Device, event.ID.Hex()))
		return
	}

	if evtdb.Pushed > 0 {
		LoggingClient.Error(fmt.Sprintf("%s Ignored event %s, Already pushed",
			event.Device, event.ID.Hex()))
		return
	}

	if reg.format == nil {
		LoggingClient.Warn("registrationInfo with nil format " + reg.registration.Name)
		return
	}
	formatted := reg.format.Format(data)

	compressed := formatted
	if reg.compression != nil {
		compressed = reg.compression.Transform(formatted)
	}

	bytes := compressed
	if reg.encrypt != nil {
		bytes = reg.encrypt.Transform(compressed)
	}
	if reg.sender.Send(bytes, ctx) == false {
		LoggingClient.Error(fmt.Sprintf("MQTT comm failure event %s", event.ID.Hex()))
		return
	}

	if Configuration.Writable.MarkPushed {
		id := event.ID.Hex()
		err := ec.MarkPushed(event.ID, ctx)
		if err != nil {
			LoggingClient.Error(fmt.Sprintf("Failed to mark event as pushed : event ID = %s: %s", id, err))
		} else {
			LoggingClient.Info(fmt.Sprintf("Marked event %s as pushed", id))
		}
	}
	LoggingClient.Debug(fmt.Sprintf("Sent event %s with registration: %s", event.ID.Hex(),
		reg.registration.Name))
	return
}

func (reg registrationInfo) handleCBOR(msg msgTypes.MessageEnvelope, ctx context.Context) (err error) {
	if reg.sender.Send(msg.Payload, ctx) && Configuration.Writable.MarkPushed {
		//Cannot use CBOR Content-Type when calling back to core-data. Ensure Content-Type is JSON.
		ctxCallback := context.WithValue(context.Background(), clients.CorrelationHeader, msg.CorrelationID)
		ctxCallback = context.WithValue(ctxCallback, clients.ContentType, clients.ContentTypeJSON)
		return ec.MarkPushedByChecksum(msg.Checksum, ctxCallback)
	}
	return
}

func registrationLoop(reg *registrationInfo) {
	LoggingClient.Info(fmt.Sprintf("registration loop started: %s", reg.registration.Name))
	for {
		select {
		case msg := <-reg.chMessages:
			if reg.registration.Enable {
				reg.processMessage(msg)
			}

		case newReg := <-reg.chRegistration:
			if newReg == nil {
				LoggingClient.Info("Terminating registration goroutine")
				return
			} else {
				if reg.update(*newReg) {
					LoggingClient.Info(fmt.Sprintf("Registration %s updated: OK", reg.registration.Name))
				} else {
					LoggingClient.Info(fmt.Sprintf("Registration %s updated: OK, terminating goroutine", reg.registration.Name))
					reg.deleteFlag = true
					return
				}
			}
		}
	}
}

func updateRunningRegistrations(running map[string]*registrationInfo,
	update contract.NotifyUpdate) error {

	switch update.Operation {
	case contract.NotifyUpdateDelete:
		for k, v := range running {
			if k == update.Name {
				v.chRegistration <- nil
				delete(running, k)
				return nil
			}
		}
		return fmt.Errorf("delete update not processed")
	case contract.NotifyUpdateUpdate:
		reg := getRegistrationByName(update.Name)
		if reg == nil {
			return fmt.Errorf("Could not find registration")
		}
		for k, v := range running {
			if k == update.Name {
				v.chRegistration <- reg
				return nil
			}
		}
		return fmt.Errorf("Could not find running registration")
	case contract.NotifyUpdateAdd:
		reg := getRegistrationByName(update.Name)
		if reg == nil {
			return fmt.Errorf("Could not find registration")
		}
		regInfo := newRegistrationInfo()
		if regInfo.update(*reg) {
			running[reg.Name] = regInfo
			go registrationLoop(regInfo)
		}
		return nil
	default:
		return fmt.Errorf("Invalid update operation")
	}
}

// Loop - registration loop
func Loop() {
	go func() {
		p := fmt.Sprintf(":%d", Configuration.Service.Port)
		LoggingClient.Info(fmt.Sprintf("Starting Export Distro %s", p))
		messageErrors <- http.ListenAndServe(p, httpServer())
	}()

	registrations := make(map[string]*registrationInfo)

	allRegs, err := getRegistrations()

	for allRegs == nil {
		LoggingClient.Info("Waiting for client microservice")
		select {
		case e := <-messageErrors:
			LoggingClient.Error(fmt.Sprintf("exit msg: %s", e.Error()))
			if err != nil {
				LoggingClient.Error(fmt.Sprintf("with error: %s", err.Error()))
			}
			return
		case <-processStop:
			LoggingClient.Info(fmt.Sprintf("received term signal"))
			return
		case <-time.After(time.Second):
		}
		allRegs, err = getRegistrations()
	}

	// Create new goroutines for each registration
	for _, reg := range allRegs {
		regInfo := newRegistrationInfo()
		if regInfo.update(reg) {
			registrations[reg.Name] = regInfo
			go registrationLoop(regInfo)
		}
	}

	LoggingClient.Info("Starting registration loop")
	for {
		select {
		case e := <-messageErrors:
			// kill all registration goroutines
			stop(registrations)
			LoggingClient.Error(fmt.Sprintf("exit msg: %s", e.Error()))
			return

		case <-processStop:
			// kill all registration goroutines
			stop(registrations)
			LoggingClient.Info(fmt.Sprintf("received term signal"))
			return

		case update := <-registrationChanges:
			LoggingClient.Info("Registration changes")
			err := updateRunningRegistrations(registrations, update)
			if err != nil {
				LoggingClient.Error(err.Error())
				LoggingClient.Warn(fmt.Sprintf("Error updating registration %s", update.Name))
			}

		case msgEnvelope := <-messageEnvelopes:
			LoggingClient.Debug("message received via bus", "Topic", Configuration.MessageQueue.Topic, clients.CorrelationHeader, msgEnvelope.CorrelationID)

			for k, reg := range registrations {
				if reg.deleteFlag {
					delete(registrations, k)
				} else {
					reg.chMessages <- msgEnvelope
				}
			}
		}
	}
}

func stop(registrations map[string]*registrationInfo) {
	for k, reg := range registrations {
		if !reg.deleteFlag {
			// Do not write in channel that will not be read
			reg.chRegistration <- nil
		}
		delete(registrations, k)
	}
}
