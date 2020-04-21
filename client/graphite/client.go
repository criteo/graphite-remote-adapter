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
	"net"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	graphiteCfg "github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/criteo/graphite-remote-adapter/client/graphite/paths"
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
	format         paths.Format

	carbonCon               net.Conn
	carbonLastReconnectTime time.Time
	carbonConLock           sync.Mutex

	logger log.Logger
}

// NewClient returns a new Client.
func NewClient(cfg *config.Config, logger log.Logger) *Client {
	if cfg.Graphite.Write.CarbonAddress == "" && cfg.Graphite.Read.URL == "" {
		return nil
	}
	if cfg.Graphite.Write.EnablePathsCache {
		paths.InitPathsCache(cfg.Graphite.Write.PathsCacheTTL,
			cfg.Graphite.Write.PathsCachePurgeInterval)
		level.Debug(logger).Log(
			"PathsCacheTTL", cfg.Graphite.Write.PathsCacheTTL,
			"PathsCachePurgeInterval", cfg.Graphite.Write.PathsCachePurgeInterval,
			"msg", "Paths cache initialized")
	}

	// Which format are we using to write points?
	format := paths.Format{Type: paths.FormatCarbon}
	if cfg.Graphite.EnableTags || cfg.Graphite.FilteredTags != "" {
		if cfg.Graphite.UseOpenMetricsFormat {
			format = paths.Format{Type: paths.FormatCarbonOpenMetrics}
		} else {
			format = paths.Format{Type: paths.FormatCarbonTags}
		}

		format.FilteredTags = strings.Split(cfg.Graphite.FilteredTags, ",")
	}

	return &Client{
		logger:       logger,
		cfg:          &cfg.Graphite,
		writeTimeout: cfg.Write.Timeout,
		format:       format,
		readTimeout:  cfg.Read.Timeout,
		readDelay:    cfg.Read.Delay,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "remote_adapter_graphite",
				Name:      "ignored_samples_total",
				Help:      "The total number of samples not sent to Graphite due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
		carbonCon:               nil,
		carbonLastReconnectTime: time.Time{},
		carbonConLock:           sync.Mutex{},
	}
}

// Shutdown the client.
func (c *Client) Shutdown() {
	c.carbonConLock.Lock()
	defer c.carbonConLock.Unlock()
	c.disconnectFromCarbon()
}

// Name implements the client.Client interface.
func (c *Client) Name() string {
	return "graphite"
}

// Target respond with a more low level representation of the client's remote
func (c *Client) Target() string {
	if c.carbonCon == nil {
		return "unknown"
	}
	return c.carbonCon.RemoteAddr().String()
}

// String implements the client.Client interface.
func (c *Client) String() string {
	// TODO: add more stuff here.
	return c.cfg.String()
}
