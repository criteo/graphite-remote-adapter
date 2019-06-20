// Copyright 2017 Thibault Chataigner <thibault.chataigner@gmail.com>
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package graphite

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/go-kit/kit/log"
)

var (
	testClient = &Client{
		logger: log.NewNopLogger(),
		cfg: &config.Config{
			DefaultPrefix: "prometheus-prefix.",
			Write:         config.WriteConfig{},
			Read: config.ReadConfig{
				URL: "http://fakeHost:6666",
			},
		},
	}
)

func TestGetGraphitePrefix(t *testing.T) {
	fakeRequest, _ := http.NewRequest("POST", "http://fakeHost:6666", nil)
	expectedPrefix := testClient.cfg.DefaultPrefix

	actualPrefix := testClient.cfg.StoragePrefixFromRequest(fakeRequest)
	if !reflect.DeepEqual(expectedPrefix, actualPrefix) {
		t.Errorf("Expected %s, got %s", expectedPrefix, actualPrefix)
	}
}

func TestGetCustomGraphitePrefix(t *testing.T) {
	fakeRequest, _ := http.NewRequest("POST", "http://fakeHost:6666?graphite.default-prefix=foo.bar.custom.", nil)
	expectedPrefix := "foo.bar.custom."

	actualPrefix := testClient.cfg.StoragePrefixFromRequest(fakeRequest)
	if !reflect.DeepEqual(expectedPrefix, actualPrefix) {
		t.Errorf("Expected %s, got %s", expectedPrefix, actualPrefix)
	}
}
