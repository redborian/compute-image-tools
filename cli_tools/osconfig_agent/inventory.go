//  Copyright 2017 Google Inc. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/compute-image-tools/go/osinfo"
	"github.com/GoogleCloudPlatform/compute-image-tools/go/packages"
)

func init() {
	packages.DebugLogger = debugLog
}

const (
	inventoryURL = reportURL + "/guestInventory"
)

type instanceInventory struct {
	Hostname          string
	LongName          string
	ShortName         string
	Version           string
	Architecture      string
	KernelVersion     string
	InstalledPackages packages.Packages
	PackageUpdates    packages.Packages
	Errors            []string
}

func postAttributeCompressed(url string, body interface{}) error {
	buf := &bytes.Buffer{}
	b := base64.NewEncoder(base64.StdEncoding, buf)
	zw := gzip.NewWriter(b)
	w := json.NewEncoder(zw)
	if err := w.Encode(body); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}
	if err := b.Close(); err != nil {
		return err
	}

	return postAttribute(url, buf)
}

func writeInventory(state *instanceInventory, url string) {
	logInfof("Writing instance inventory.")

	if err := postAttribute(url+"/Timestamp", strings.NewReader(time.Now().UTC().Format(time.RFC3339))); err != nil {
		state.Errors = append(state.Errors, err.Error())
		logErrorf("postAttribute error: %v", err)
	}

	e := reflect.ValueOf(state).Elem()
	t := e.Type()
	for i := 0; i < e.NumField(); i++ {
		f := e.Field(i)
		u := fmt.Sprintf("%s/%s", url, t.Field(i).Name)
		logDebugf("postAttribute %s: %+v\n", u, f)
		switch f.Kind() {
		case reflect.String:
			if err := postAttribute(u, strings.NewReader(f.String())); err != nil {
				state.Errors = append(state.Errors, err.Error())
				logErrorf("postAttribute error: %v", err)
			}
		case reflect.Struct:
			if err := postAttributeCompressed(u, f.Interface()); err != nil {
				state.Errors = append(state.Errors, err.Error())
				logErrorf("postAttributeCompressed error: %v", err)
			}
		}
	}
	if err := postAttribute(url+"/Errors", strings.NewReader(fmt.Sprintf("%q", state.Errors))); err != nil {
		logErrorf("postAttribute error: %v", err)
	}
}

func getInventory() *instanceInventory {
	logInfof("Gathering instance inventory.")

	hs := &instanceInventory{}

	hn, err := os.Hostname()
	if err != nil {
		hs.Errors = append(hs.Errors, err.Error())
	}

	hs.Hostname = hn

	di, err := osinfo.GetDistributionInfo()
	if err != nil {
		hs.Errors = append(hs.Errors, err.Error())
	}

	hs.LongName = di.LongName
	hs.ShortName = di.ShortName
	hs.Version = di.Version
	hs.KernelVersion = di.Kernel
	hs.Architecture = di.Architecture

	var errs []string
	hs.InstalledPackages, errs = packages.GetInstalledPackages()
	if len(errs) != 0 {
		hs.Errors = append(hs.Errors, errs...)
	}

	hs.PackageUpdates, errs = packages.GetPackageUpdates()
	if len(errs) != 0 {
		hs.Errors = append(hs.Errors, errs...)
	}

	return hs
}

func runInventory() {
	writeInventory(getInventory(), inventoryURL)
}
