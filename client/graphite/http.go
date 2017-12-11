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
	"encoding/json"

	"github.com/criteo/graphite-remote-adapter/utils"
)

// make it mockable in tests
var (
	fetchURL   = utils.FetchURL
	prepareURL = utils.PrepareURL
)

// ExpandResponse is a parsed response of graphite expand endpoint.
type ExpandResponse struct {
	Results []string `yaml:"results,omitempty" json:"results,omitempty"`
}

// RenderResponse is a single parsed element of graphite render endpoint.
type RenderResponse struct {
	Target     string       `yaml:"target,omitempty" json:"target,omitempty"`
	Datapoints []*Datapoint `yaml:"datapoints,omitempty" json:"datapoints,omitempty"`
}

// Datapoint pairs a timestamp to a value.
type Datapoint struct {
	Value     *float64
	Timestamp int64
}

// UnmarshalJSON unmarshals a Datapoint from json
func (d *Datapoint) UnmarshalJSON(b []byte) error {
	var x []*interface{}
	err := json.Unmarshal(b, &x)
	if err != nil {
		return err
	}
	if x[0] != nil {
		val := new(float64)
		*val = (*x[0]).(float64)
		d.Value = val
	}
	d.Timestamp = int64((*x[1]).(float64))
	return nil
}
