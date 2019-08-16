// -----------------------------------------------------------------------------
//  Copyright (c)
//  2017 Mainflux
//  2019 Infiot Inc.
//  All Rights Reserved.
//  SPDX-License-Identifier: Apache-2.0
// -----------------------------------------------------------------------------

package distro

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/edgexfoundry/go-mod-core-contracts/models"
)

const (
	tcpsPrefix    = "tcps"
	sslPrefix     = "ssl"
	tlsPrefix     = "tls"
	devicesPrefix = "/devices/"
)

type iotCoreSender struct {
	client MQTT.Client
	topic  string
}

func secretsPresent(pkey string, cert string) (status bool) {
	LoggingClient.Info(fmt.Sprintf("Checking for existence of %v:%v\n", pkey, cert))
	_, err := os.Stat(pkey)
	if err != nil {
		return false
	}

	_, err = os.Stat(cert)
	if err != nil {
		return false
	}

	return true
}

// newIoTCoreSender returns new Google IoT Core sender instance.
func newIoTCoreSender(addr models.Addressable) sender {
	protocol := strings.ToLower(addr.Protocol)
	broker := fmt.Sprintf("%s%s", addr.GetBaseURL(), addr.Path)
	deviceID := extractDeviceID(addr.Publisher)

	opts := MQTT.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(addr.Publisher)
	opts.SetUsername(addr.User)
	opts.SetPassword(addr.Password)
	opts.SetAutoReconnect(false)

	if validateProtocol(protocol) {
		c := Configuration.Certificates["MQTTS"]

		//Don't bail out if secrets (cert, private key)
		//are not present
		if secretsPresent(c.Cert, c.Key) == true {
			cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
			if err != nil {
				LoggingClient.Error("Failed loading x509 data")
				return nil
			}

			opts.SetTLSConfig(&tls.Config{
				ClientCAs:          nil,
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{cert},
			})
		} else {
			LoggingClient.Info("Keys not present, Setting TLS config accordingly\n")
			opts.SetTLSConfig(&tls.Config{
				ClientCAs:          nil,
				InsecureSkipVerify: true,
				Certificates:       nil,
			})
		}
	}

	if addr.Topic == "" {
		addr.Topic = fmt.Sprintf("/devices/%s/events", deviceID)
	}

	LoggingClient.Info(fmt.Sprintf("Topic: %s, Url: %s\n",
		addr.Topic, broker))
	return &mqttSender{
		client: MQTT.NewClient(opts),
		topic:  addr.Topic,
	}
}

func (sender *iotCoreSender) Send(data []byte) bool {
	if !sender.client.IsConnected() {
		LoggingClient.Info("Connecting to IoT core mqtt server")
		token := sender.client.Connect()
		token.Wait()
		if token.Error() != nil {
			LoggingClient.Error(fmt.Sprintf("Couldn't connect to IoT core, drop event. Error: %s",
				token.Error().Error()))
			return false
		}
	}

	token := sender.client.Publish(sender.topic, 0, false, data)
	token.Wait()
	if token.Error() != nil {
		LoggingClient.Error(token.Error().Error())
		return false
	}

	LoggingClient.Debug(fmt.Sprintf("Sent data: %X", data))
	return true
}

func extractDeviceID(addr string) string {
	return addr[strings.Index(addr, devicesPrefix)+len(devicesPrefix):]
}

func validateProtocol(protocol string) bool {
	return protocol == tcpsPrefix || protocol == sslPrefix || protocol == tlsPrefix
}
