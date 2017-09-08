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
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/remote"

	"golang.org/x/net/context"

	"github.com/criteo/graphite-remote-adapter/graphite/config"
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
	read_delay       time.Duration
	prefix           string
	config           *config.Config
	rules            []*config.Rule
	template_data    map[string]interface{}
	ignoredSamples   prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(carbon string, carbon_transport string, write_timeout time.Duration,
	graphite_web string, read_timeout time.Duration, prefix string, configFile string,
	read_delay time.Duration, usePathsCache bool, pathsCacheExpiration time.Duration,
	pathsCachePurge time.Duration) *Client {
	fileConf := &config.Config{}
	if configFile != "" {
		var err error
		fileConf, err = config.LoadFile(configFile)
		if err != nil {
			log.With("err", err).Warnln("Error loading config file")
			return nil
		}
	}
	if usePathsCache {
		initPathsCache(pathsCacheExpiration, pathsCachePurge)
	}
	return &Client{
		carbon:           carbon,
		carbon_transport: carbon_transport,
		write_timeout:    write_timeout,
		graphite_web:     graphite_web,
		read_timeout:     read_timeout,
		read_delay:       read_delay,
		prefix:           prefix,
		config:           fileConf,
		rules:            fileConf.Rules,
		template_data:    fileConf.Template_data,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_graphite_ignored_samples_total",
				Help: "The total number of samples not sent to Graphite due to unsupported float values (Inf, -Inf, NaN).",
			},
		),
	}
}

func (c *Client) prepareDataPoint(path string, s *model.Sample) string {
	t := float64(s.Timestamp.UnixNano()) / 1e9
	v := float64(s.Value)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		log.Debugf("cannot send value %f to Graphite,"+
			"skipping sample %#v", v, s)
		c.ignoredSamples.Inc()
		return ""
	}
	return fmt.Sprintf("%s %f %f\n", path, v, t)
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
		paths := pathsFromMetric(s.Metric, c.prefix, c.rules, c.template_data)
		for _, k := range paths {
			if str := c.prepareDataPoint(k, s); str != "" {
				fmt.Fprint(&buf, str)
			}
		}
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) queryToTargets(query *remote.Query, ctx context.Context) ([]string, error) {
	// Parse metric name from query
	var name string
	for _, labelMatcher := range query.Matchers {
		if labelMatcher.Name == model.MetricNameLabel && remote.MatchType_name[int32(labelMatcher.Type)] == "EQUAL" {
			name = labelMatcher.Value
		}
	}
	if name == "" {
		err := fmt.Errorf("Invalide remote query: no %s label provided", model.MetricNameLabel)
		return nil, err
	}

	// Prepare the url to fetch
	queryStr := c.prefix + name + ".**"
	expandUrl := prepareUrl(c.graphite_web, expandEndpoint, map[string]string{"format": "json", "leavesOnly": "1", "query": queryStr})

	// Get the list of targets
	expandResponse := ExpandResponse{}
	body, err := fetchUrl(expandUrl, ctx)
	err = json.Unmarshal(body, &expandResponse)
	if err != nil {
		log.With("url", expandUrl).With("err", err).Warnln("Error parsing expand endpoint response body")
		return nil, err
	}
	return expandResponse.Results, nil
}

func (c *Client) targetToTimeseries(target string, from string, until string, ctx context.Context) (*remote.TimeSeries, error) {
	renderUrl := prepareUrl(c.graphite_web, renderEndpoint, map[string]string{"format": "json", "from": from, "until": until, "target": target})
	renderResponses := make([]RenderResponse, 0)
	body, err := fetchUrl(renderUrl, ctx)
	err = json.Unmarshal(body, &renderResponses)
	if err != nil {
		log.With("url", renderUrl).With("err", err).Warnln("Error parsing render endpoint response body")
		return nil, err
	}
	renderResponse := renderResponses[0]

	ts := &remote.TimeSeries{}
	ts.Labels = metricLabelsFromPath(renderResponse.Target, c.prefix)
	for _, datapoint := range renderResponse.Datapoints {
		timstamp_ms := datapoint.Timestamp * 1000
		if datapoint.Value == nil {
			continue
		}
		ts.Samples = append(ts.Samples, &remote.Sample{Value: *datapoint.Value, TimestampMs: timstamp_ms})
	}
	return ts, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) handleReadQuery(query *remote.Query, ctx context.Context) (*remote.QueryResult, error) {
	queryResult := &remote.QueryResult{}

	now := int(time.Now().Unix())
	from := int(query.StartTimestampMs / 1000)
	until := int(query.EndTimestampMs / 1000)
	delta := int(c.read_delay.Seconds())
	until = min(now-delta, until)

	if until < from {
		log.Debugf("Skipping query with empty time range")
		return queryResult, nil
	}
	from_str := strconv.Itoa(from)
	until_str := strconv.Itoa(until)

	targets, err := c.queryToTargets(query, ctx)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		ts, err := c.targetToTimeseries(target, from_str, until_str, ctx)
		if err != nil {
			log.With("target", target).With("err", err).Warnln("Error fetching and parsing target datapoints")
			continue
		}
		queryResult.Timeseries = append(queryResult.Timeseries, ts)
	}
	return queryResult, nil
}

func (c *Client) Read(req *remote.ReadRequest) (*remote.ReadResponse, error) {
	log.With("req", req).Debugf("Remote read")

	if c.graphite_web == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.read_timeout)
	defer cancel()

	resp := &remote.ReadResponse{}
	for _, query := range req.Queries {
		queryResult, err := c.handleReadQuery(query, ctx)
		if err != nil {
			return nil, err
		}
		resp.Results = append(resp.Results, queryResult)
	}
	return resp, nil
}

// Name identifies the client as a Graphite client.
func (c Client) Name() string {
	return "graphite"
}

func (c Client) String() string {
	// TODO: add more stuff here.
	return c.config.String()
}
