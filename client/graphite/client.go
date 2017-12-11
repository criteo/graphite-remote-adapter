// Copyright 2015 The Prometheus Authors
// Copyright 2017 Corentin Chary <corentin.chary@gmail.com>
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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	graphiteCfg "github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/criteo/graphite-remote-adapter/config"
)

const (
	expandEndpoint  = "/metrics/expand"
	renderEndpoint  = "/render/"
	maxFetchWorkers = 10
)

// Client allows sending batches of Prometheus samples to Graphite.
type Client struct {
	lock           sync.RWMutex
	cfg            *graphiteCfg.Config
	writeTimeout   time.Duration
	readTimeout    time.Duration
	readDelay      time.Duration
	ignoredSamples prometheus.Counter

	logger log.Logger
}

// NewClient returns a new Client.
func NewClient(cfg *config.Config, logger log.Logger) *Client {
	if cfg.Graphite.Write.CarbonAddress == "" && cfg.Graphite.Read.URL == "" {
		return nil
	}
	if cfg.Graphite.Write.EnablePathsCache {
		initPathsCache(cfg.Graphite.Write.PathsCacheTTL,
			cfg.Graphite.Write.PathsCachePurgeInterval)
		level.Debug(logger).Log(
			"PathsCacheTTL", cfg.Graphite.Write.PathsCacheTTL,
			"PathsCachePurgeInterval", cfg.Graphite.Write.PathsCachePurgeInterval,
			"msg", "Paths cache initialized")
	}
	return &Client{
		logger:       logger,
		cfg:          &cfg.Graphite,
		writeTimeout: cfg.Write.Timeout,
		readTimeout:  cfg.Read.Timeout,
		readDelay:    cfg.Read.Delay,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_graphite_ignored_samples_total",
				Help: "The total number of samples not sent to Graphite due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}
}

// Name implements the client.Client interface.
func (c *Client) Name() string {
	return "graphite"
}

// String implements the client.Client interface.
func (c *Client) String() string {
	// TODO: add more stuff here.
	return c.cfg.String()
}
