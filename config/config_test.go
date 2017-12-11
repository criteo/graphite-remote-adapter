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

package config

import (
	"testing"
	"time"

	graphite "github.com/criteo/graphite-remote-adapter/graphite/config"
	"github.com/go-kit/kit/log"
)

var expectedConf = &Config{
	Web: webOptions{
		ListenAddress: "1.2.3.4:666",
		TelemetryPath: "/coolMetrics",
	},
	Read: readOptions{
		Timeout:     18 * time.Minute,
		Delay:       42 * time.Minute,
		IgnoreError: true,
	},
	Write: writeOptions{
		Timeout: 18 * time.Minute,
	},
	Graphite: graphite.DefaultConfig,
	original: "",
}

func TestLoadConfigFile(t *testing.T) {
	c, err := LoadFile(log.NewNopLogger(), "testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}
	c.original = ""

	if c.String() != expectedConf.String() {
		t.Fatalf("%s: unexpected config result: \n%s\nExpecting:\n%s",
			"testdata/conf.good.yml", c.String(), expectedConf.String())
	}
}
