//
// Copyright (c) 2017
// Cavium
// Mainflux
// IOTech
//
// SPDX-License-Identifier: Apache-2.0
//

package distro

import (
	"context"
	"crypto/tls"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	contract "github.com/edgexfoundry/go-mod-core-contracts/models"
)

type mqttSender struct {
	client                MQTT.Client
	topic                 string
	lastMsgPublished      time.Time
	firstMsgPublished     bool
	minDeltaBetweenMsgs   time.Duration
	rateLimit             bool
}

// newMqttSender - create new mqtt sender
func newMqttSender(addr contract.Addressable, cert string, key string) sender {
	protocol := strings.ToLower(addr.Protocol)

	opts := MQTT.NewClientOptions()
	broker := protocol + "://" + addr.Address + ":" + strconv.Itoa(addr.Port) + addr.Path
	opts.AddBroker(broker)
	opts.SetClientID(addr.Publisher)
	opts.SetUsername(addr.User)
	opts.SetPassword(addr.Password)
	opts.SetAutoReconnect(false)
	opts.SetWriteTimeout(30 * time.Second)

	if protocol == "tcps" || protocol == "ssl" || protocol == "tls" {
		cert, err := tls.LoadX509KeyPair(cert, key)

		if err != nil {
			LoggingClient.Error("Failed loading x509 data")
			return nil
		}

		tlsConfig := &tls.Config{
			ClientCAs:          nil,
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{cert},
		}

		opts.SetTLSConfig(tlsConfig)

	}

	sender := &mqttSender{
		client: MQTT.NewClient(opts),
		topic:  addr.Topic,
		lastMsgPublished: time.Now(),
		firstMsgPublished: false,
		rateLimit: false,
		minDeltaBetweenMsgs: 0,
	}

	// TODO: Make it generic
	if filepath.Base(sender.topic) == "state" {
		sender.rateLimit = true
		sender.minDeltaBetweenMsgs, _ = time.ParseDuration("1s")
		LoggingClient.Info(fmt.Sprintf("Topic %s will be rate limited", sender.topic))
	} else {
		LoggingClient.Info(fmt.Sprintf("Topic %s will not be rate limited", sender.topic))
	}

	return sender
}

func (sender *mqttSender) Send(data []byte, ctx context.Context) bool {
	if !sender.client.IsConnected() {
		LoggingClient.Info(fmt.Sprintf("%s Connecting to mqtt server", sender.topic))
		if token := sender.client.Connect(); token.Wait() && token.Error() != nil {
			LoggingClient.Error(fmt.Sprintf("%s Could not connect to mqtt server, drop event. Error: %s",
				sender.topic, token.Error().Error()))
			return false
		}
	}

	if sender.firstMsgPublished && sender.rateLimit {
		timeElapsedLastMessage := time.Now().Sub(sender.lastMsgPublished)
		for {
			if timeElapsedLastMessage < sender.minDeltaBetweenMsgs {
				//LoggingClient.Info(
				//	fmt.Sprintf("%s Delaying processing event %s due to rate limit",
				//	sender.topic, event.ID.Hex()))
				LoggingClient.Info(
					fmt.Sprintf("%s Delaying processing event due to rate limit",
					sender.topic))
					
				time.Sleep(500 * time.Millisecond)
				timeElapsedLastMessage = time.Now().Sub(sender.lastMsgPublished)
				continue
			}
			break
		}
	}

	sender.lastMsgPublished = time.Now()
	sender.firstMsgPublished = true

	LoggingClient.Debug(fmt.Sprintf("%s Pre Publishing data %d bytes", sender.topic, len(data)))
	token := sender.client.Publish(sender.topic, 0, false, data)
	LoggingClient.Debug(fmt.Sprintf("%s Post Publishing data %d bytes", sender.topic, len(data)))
	// FIXME: could be removed? set of tokens?
	respReceived := token.WaitTimeout(5 * time.Second)
	if respReceived {
		if token.Error() != nil {
			LoggingClient.Error(token.Error().Error())
			return false
		} else {
			LoggingClient.Debug(fmt.Sprintf("%s Sent data: %d bytes", sender.topic, len(data)))
			return true
		}
	} else {
		// This is fatal
		LoggingClient.Error(fmt.Sprintf("%s MQTT timeout", sender.topic))
		return false
	}
}
