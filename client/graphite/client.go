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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	pmetric "github.com/prometheus/prometheus/storage/metric"

	"golang.org/x/net/context"

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

// NewClient creates a new Client.
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

func (c *Client) prepareDataPoint(path string, s *model.Sample) string {
	t := float64(s.Timestamp.UnixNano()) / 1e9
	v := float64(s.Value)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		level.Debug(c.logger).Log(
			"value", v, "sample", s, "msg", "cannot send a value, skipping sample")
		c.ignoredSamples.Inc()
		return ""
	}
	return fmt.Sprintf("%s %f %f\n", path, v, t)
}

// Write sends a batch of samples to Graphite.
func (c *Client) Write(samples model.Samples) error {
	level.Debug(c.logger).Log(
		"num_samples", len(samples), "storage", c.Name(), "msg", "Remote write")
	if c.cfg.Write.CarbonAddress == "" {
		return nil
	}

	conn, err := net.DialTimeout(c.cfg.Write.CarbonTransport, c.cfg.Write.CarbonAddress, c.writeTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, s := range samples {
		paths := pathsFromMetric(s.Metric, c.cfg.DefaultPrefix, c.cfg.Write.Rules, c.cfg.Write.TemplateData)
		for _, k := range paths {
			if str := c.prepareDataPoint(k, s); str != "" {
				fmt.Fprint(&buf, str)
				level.Debug(c.logger).Log("line", str, "msg", "Sending")
			}
		}
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) queryToTargets(query *prompb.Query, ctx context.Context) ([]string, error) {
	// Parse metric name from query
	var name string

	for _, m := range query.Matchers {
		if m.Name == model.MetricNameLabel && m.Type == prompb.LabelMatcher_EQ {
			name = m.Value
		}
	}

	if name == "" {
		err := fmt.Errorf("Invalide remote query: no %s label provided", model.MetricNameLabel)
		return nil, err
	}

	// Prepare the url to fetch
	queryStr := c.cfg.DefaultPrefix + name + ".**"
	expandUrl, err := prepareURL(c.cfg.Read.URL, expandEndpoint, map[string]string{"format": "json", "leavesOnly": "1", "query": queryStr})
	if err != nil {
		level.Warn(c.logger).Log(
			"graphite_web", c.cfg.Read.URL, "path", expandEndpoint,
			"err", err, "msg", "Error preparing URL")
		return nil, err
	}

	// Get the list of targets
	expandResponse := ExpandResponse{}
	body, err := fetchURL(c.logger, expandUrl, ctx)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", expandUrl, "err", err, "msg", "Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &expandResponse)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", expandUrl, "err", err,
			"msg", "Error parsing expand endpoint response body")
		return nil, err
	}

	targets, err := c.filterTargets(query, expandResponse.Results)
	return targets, err
}

func (c *Client) filterTargets(query *prompb.Query, targets []string) ([]string, error) {
	// Filter out targets that do not match the query's label matcher
	var results []string
	for _, target := range targets {
		// Put labels in a map.
		labels, err := metricLabelsFromPath(target, c.cfg.DefaultPrefix)
		if err != nil {
			level.Warn(c.logger).Log(
				"path", target, "prefix", c.cfg.DefaultPrefix, "err", err)
			continue
		}
		labelSet := make(model.LabelSet, len(labels))

		for _, label := range labels {
			labelSet[model.LabelName(label.Name)] = model.LabelValue(label.Value)
		}

		level.Debug(c.logger).Log(
			"target", target, "prefix", c.cfg.DefaultPrefix,
			"labels", labels, "msg", "Filtering target")

		// See if all matchers are satisfied.
		match := true
		for _, m := range query.Matchers {
			matcher, err := pmetric.NewLabelMatcher(
				pmetric.MatchType(m.Type), model.LabelName(m.Name), model.LabelValue(m.Value))
			if err != nil {
				return nil, err
			}

			if !matcher.Match(labelSet[model.LabelName(m.Name)]) {
				match = false
				break
			}
		}

		// If everything is fine, keep this target.
		if match {
			results = append(results, target)
		}
	}
	return results, nil
}

func (c *Client) targetToTimeseries(target string, from string, until string, ctx context.Context) (*prompb.TimeSeries, error) {
	renderUrl, err := prepareURL(c.cfg.Read.URL, renderEndpoint, map[string]string{"format": "json", "from": from, "until": until, "target": target})
	if err != nil {
		level.Warn(c.logger).Log(
			"graphite_web", c.cfg.Read.URL, "path", renderEndpoint,
			"err", err, "msg", "Error preparing URL")
		return nil, err
	}

	renderResponses := make([]RenderResponse, 0)
	body, err := fetchURL(c.logger, renderUrl, ctx)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", renderUrl, "err", err, "msg", "Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &renderResponses)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", renderUrl, "err", err,
			"msg", "Error parsing render endpoint response body")
		return nil, err
	}
	renderResponse := renderResponses[0]

	ts := &prompb.TimeSeries{}
	ts.Labels, err = metricLabelsFromPath(renderResponse.Target, c.cfg.DefaultPrefix)
	if err != nil {
		level.Warn(c.logger).Log(
			"path", renderResponse.Target, "prefix", c.cfg.DefaultPrefix, "err", err)
		return nil, err
	}
	for _, datapoint := range renderResponse.Datapoints {
		timstamp_ms := datapoint.Timestamp * 1000
		if datapoint.Value == nil {
			continue
		}
		ts.Samples = append(ts.Samples, &prompb.Sample{Value: *datapoint.Value, Timestamp: timstamp_ms})
	}
	return ts, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) handleReadQuery(query *prompb.Query, ctx context.Context) (*prompb.QueryResult, error) {
	queryResult := &prompb.QueryResult{}

	now := int(time.Now().Unix())
	from := int(query.StartTimestampMs / 1000)
	until := int(query.EndTimestampMs / 1000)
	delta := int(c.readDelay.Seconds())
	until = min(now-delta, until)

	if until < from {
		level.Debug(c.logger).Log("msg", "Skipping query with empty time range")
		return queryResult, nil
	}
	from_str := strconv.Itoa(from)
	until_str := strconv.Itoa(until)

	targets, err := c.queryToTargets(query, ctx)
	if err != nil {
		return nil, err
	}

	c.fetchData(queryResult, targets, ctx, from_str, until_str)
	return queryResult, nil
}

func (c *Client) fetchData(queryResult *prompb.QueryResult, targets []string, ctx context.Context, from_str string, until_str string) {
	input := make(chan string, len(targets))
	output := make(chan *prompb.TimeSeries, len(targets))

	wg := sync.WaitGroup{}

	// Start only a few workers to avoid killing graphite.
	for i := 0; i < maxFetchWorkers; i++ {
		wg.Add(1)

		go func(from_str string, until_str string, ctx context.Context) {
			defer wg.Done()

			for target := range input {
				// We simply ignore errors here as it is better to return "some" data
				// than nothing.
				ts, err := c.targetToTimeseries(target, from_str, until_str, ctx)
				if err != nil {
					level.Warn(c.logger).Log("target", target, "err", err, "msg", "Error fetching and parsing target datapoints")
				} else {
					output <- ts
				}
			}
		}(from_str, until_str, ctx)
	}

	// Feed the inut.
	for _, target := range targets {
		input <- target
	}
	close(input)

	wg.Wait()
	close(output)

	for ts := range output {
		queryResult.Timeseries = append(queryResult.Timeseries, ts)
	}
}

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	level.Debug(c.logger).Log("req", req, "msg", "Remote read")

	if c.cfg.Read.URL == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.readTimeout)
	defer cancel()

	resp := &prompb.ReadResponse{}
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
func (c *Client) Name() string {
	return "graphite"
}

func (c *Client) String() string {
	// TODO: add more stuff here.
	return c.cfg.String()
}
