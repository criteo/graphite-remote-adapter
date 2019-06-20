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

package client

import (
	"net/http"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

// Client define a remote storage.
type Client interface {
	Name() string
	Target() string
	String() string
	Shutdown()
}

// Writer is a client that sends a batch of samples to remote.
type Writer interface {
	Write(samples model.Samples, r *http.Request, dryRun bool) ([]byte, error)
	Client
}

// Reader is a client that read samples from remote.
type Reader interface {
	Read(req *prompb.ReadRequest, r *http.Request) (*prompb.ReadResponse, error)
	Client
}
