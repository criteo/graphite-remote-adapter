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

package utils

import (
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/prometheus/common/log"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// PrepareURL return an url.URL from it's parameters
func PrepareURL(scheme_host string, path string, params map[string]string) (*url.URL, error) {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	u, err := url.Parse(scheme_host)
	if err != nil {
		return nil, err
	}

	u.ForceQuery = true
	u.Path = path
	u.RawQuery = values.Encode()

	return u, nil
}

// FetchURL return body of a fetched url.URL
func FetchURL(u *url.URL, ctx context.Context) ([]byte, error) {
	log.With("url", u).With("context", ctx).Debugln("Fetching URL")

	hresp, err := ctxhttp.Get(ctx, http.DefaultClient, u.String())
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	body, err := ioutil.ReadAll(hresp.Body)
	log.With("len(body)", len(body)).With("err", err).Debugln("Fetched")
	if err != nil {
		return nil, err
	}

	return body, nil
}
