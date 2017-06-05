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
	"bytes"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/remote"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	expandEndpoint = "/metrics/expand"
	renderEndpoint = "/render/"
)

// Client allows sending batches of Prometheus samples to Graphite.
type Client struct {
	carbon           string
	carbon_transport string
	write_timeout    time.Duration
	graphite_web     string
	read_timeout     time.Duration
	prefix           string
	ignoredSamples   prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(carbon string, carbon_transport string, write_timeout time.Duration, graphite_web string, read_timeout time.Duration, prefix string) *Client {
	return &Client{
		carbon:           carbon,
		carbon_transport: carbon_transport,
		write_timeout:    write_timeout,
		graphite_web:     graphite_web,
		read_timeout:     read_timeout,
		prefix:           prefix,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_influxdb_ignored_samples_total",
				Help: "The total number of samples not sent to InfluxDB due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}
}

func pathFromMetric(m model.Metric, prefix string) string {
	var buffer bytes.Buffer

	buffer.WriteString(prefix)
	buffer.WriteString(escape(m[model.MetricNameLabel]))

	// We want to sort the labels.
	labels := make(model.LabelNames, 0, len(m))
	for l := range m {
		labels = append(labels, l)
	}
	sort.Sort(labels)

	// For each label, in order, add ".<label>.<value>".
	for _, l := range labels {
		v := m[l]

		if l == model.MetricNameLabel || len(l) == 0 {
			continue
		}
		// Since we use '.' instead of '=' to separate label and values
		// it means that we can't have an '.' in the metric name. Fortunately
		// this is prohibited in prometheus metrics.
		buffer.WriteString(fmt.Sprintf(
			".%s.%s", string(l), escape(v)))
	}
	return buffer.String()
}

// Write sends a batch of samples to Graphite.
func (c *Client) Write(samples model.Samples) error {
	log.With("num_samples", len(samples)).With("storage", c.Name()).Debugf("Remote write")
	if c.carbon == "" {
		return nil
	}

	conn, err := net.DialTimeout(c.carbon_transport, c.carbon, c.write_timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, s := range samples {
		k := pathFromMetric(s.Metric, c.prefix)
		t := float64(s.Timestamp.UnixNano()) / 1e9
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Debugf("cannot send value %f to Graphite,"+
				"skipping sample %#v", v, s)
			c.ignoredSamples.Inc()
			continue
		}
		fmt.Fprintf(&buf, "%s %f %f\n", k, v, t)
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Read(req *remote.ReadRequest) (*remote.ReadResponse, error) {
	log.With("req", req).Debugf("Remote read")

	if c.graphite_web == "" {
		return nil, nil
	}

	u, err := url.Parse(c.graphite_web)
	if err != nil {
		return nil, err
	}

	u.Path = expandEndpoint
	// TODO: set the query params correctly:
	// - format=treejson
	// - leavesOnly=1
	// - query=<prefix>.<__name__>.**

	ctx, cancel := context.WithTimeout(context.Background(), c.read_timeout)
	defer cancel()

	hresp, err := ctxhttp.Get(ctx, http.DefaultClient, u.String())
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	// TODO: Do post-filtering here and filter the right names, build TimeSeries.
	// TODO: For each metric, get data (http request to /render?format=json)
	// TODO: Parse data and build Samples.

	resp := remote.ReadResponse{
		Results: []*remote.QueryResult{
			{Timeseries: make([]*remote.TimeSeries, 0, 0)},
		},
	}

	return &resp, nil
}

// Name identifies the client as a Graphite client.
func (c Client) Name() string {
	return "graphite"
}
