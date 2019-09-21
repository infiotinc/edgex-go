/*******************************************************************************
 * Copyright 2017 Dell Inc.
 * Copyright (c) 2019 Intel Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License
 * is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
 * or implied. See the License for the specific language governing permissions and limitations under
 * the License.
 *******************************************************************************/
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/edgexfoundry/edgex-go"
	"github.com/edgexfoundry/edgex-go/internal"
	"github.com/edgexfoundry/edgex-go/internal/core/data"
	"github.com/edgexfoundry/edgex-go/internal/pkg/correlation"
	"github.com/edgexfoundry/edgex-go/internal/pkg/startup"
	"github.com/edgexfoundry/edgex-go/internal/pkg/usage"
	"github.com/edgexfoundry/go-mod-core-contracts/clients"
	"github.com/edgexfoundry/go-mod-core-contracts/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/models"
	"github.com/gorilla/context"
)

func main() {
	start := time.Now()
	var useRegistry bool
	var useProfile string

	flag.BoolVar(&useRegistry, "registry", false, "Indicates the service should use registry service.")
	flag.BoolVar(&useRegistry, "r", false, "Indicates the service should use registry service.")
	flag.StringVar(&useProfile, "profile", "", "Specify a profile other than default.")
	flag.StringVar(&useProfile, "p", "", "Specify a profile other than default.")
	flag.Usage = usage.HelpCallback
	flag.Parse()

	params := startup.BootParams{UseRegistry: useRegistry, UseProfile: useProfile, BootTimeout: internal.BootTimeoutDefault}
	startup.Bootstrap(params, data.Retry, logBeforeInit)

	ok := data.Init(useRegistry)
	if !ok {
		logBeforeInit(fmt.Errorf("%s: Service bootstrap failed!", clients.CoreDataServiceKey))
		os.Exit(1)
	}

	data.LoggingClient.Info("Service dependencies resolved...")
	data.LoggingClient.Info(fmt.Sprintf("Starting %s %s ", clients.CoreDataServiceKey, edgex.Version))

	http.TimeoutHandler(nil, time.Millisecond*time.Duration(data.Configuration.Service.Timeout), "Request timed out")
	data.LoggingClient.Info(data.Configuration.Service.StartupMsg)

	errs := make(chan error, 2)
	listenForInterrupt(errs)
	startHttpServer(errs, data.Configuration.Service.Host, data.Configuration.Service.Port)

	// Time it took to start service
	data.LoggingClient.Info("Service started in: " + time.Since(start).String())
	data.LoggingClient.Info("Listening on port: " + strconv.Itoa(data.Configuration.Service.Port))
	data.LoggingClient.Info("Listening on Host: " + data.Configuration.Service.Host)

	c := <-errs
	data.Destruct()
	data.LoggingClient.Warn(fmt.Sprintf("terminating: %v", c))

	os.Exit(0)
}

func logBeforeInit(err error) {
	l := logger.NewClient(clients.CoreDataServiceKey, false, "", models.InfoLog)
	l.Error(err.Error())
}

func listenForInterrupt(errChan chan error) {
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt)
		errChan <- fmt.Errorf("%s", <-c)
	}()
}

func startHttpServer(errChan chan error, host string, port int) {
	go func() {
		correlation.LoggingClient = data.LoggingClient //Not thrilled about this, can't think of anything better ATM
		r := data.LoadRestRoutes()
		//errChan <- http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), context.ClearHandler(r))
		errChan <- http.ListenAndServe(host+":"+strconv.Itoa(port), context.ClearHandler(r))

	}()
}
