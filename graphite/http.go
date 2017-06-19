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
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

func prepareUrl(host string, path string, params map[string]string) *url.URL {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return &url.URL{
		Scheme:     "http",
		Host:       host,
		Path:       path,
		ForceQuery: true,
		RawQuery:   values.Encode(),
	}
}

func fetchUrl(u *url.URL, ctx context.Context) ([]byte, error) {
	// TODO (t.chataigner) Add support for basic auth + proxy
	hresp, err := ctxhttp.Get(ctx, http.DefaultClient, u.String())
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	body, err := ioutil.ReadAll(hresp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

type ExpandResponse struct {
	Results []string `yaml:"results,omitempty" json:"results,omitempty"`
}

type RenderResponse struct {
	Target     string       `yaml:"target,omitempty" json:"target,omitempty"`
	Datapoints []*Datapoint `yaml:"datapoints,omitempty" json:"datapoints,omitempty"`
}

type Datapoint struct {
	Value     *float64
	Timestamp int64
}

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
