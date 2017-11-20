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

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	pmetric "github.com/prometheus/prometheus/storage/metric"

	"golang.org/x/net/context"

	"github.com/criteo/graphite-remote-adapter/graphite/config"
)

const (
	expandEndpoint = "/metrics/expand"
	renderEndpoint = "/render/"
)

// Client allows sending batches of Prometheus samples to Graphite.
type Client struct {
	lock             sync.RWMutex
	carbon           string
	carbon_transport string
	write_timeout    time.Duration
	graphite_web     string
	read_timeout     time.Duration
	read_delay       time.Duration
	prefix           string
	config           *config.Config
	ignoredSamples   prometheus.Counter
}

// NewClient creates a new Client.
func NewClient(carbon string, carbon_transport string, write_timeout time.Duration,
	graphite_web string, read_timeout time.Duration, prefix string,
	read_delay time.Duration, usePathsCache bool, pathsCacheExpiration time.Duration,
	pathsCachePurge time.Duration) *Client {
	if usePathsCache {
		initPathsCache(pathsCacheExpiration, pathsCachePurge)
	}
	// Default empty config.
	fileConf := &config.Config{}
	return &Client{
		carbon:           carbon,
		carbon_transport: carbon_transport,
		write_timeout:    write_timeout,
		graphite_web:     graphite_web,
		read_timeout:     read_timeout,
		read_delay:       read_delay,
		prefix:           prefix,
		config:           fileConf,
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
	c.lock.RLock()
	defer c.lock.RUnlock()

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
		paths := pathsFromMetric(s.Metric, c.prefix, c.config.Rules, c.config.Template_data)
		for _, k := range paths {
			if str := c.prepareDataPoint(k, s); str != "" {
				fmt.Fprint(&buf, str)
				log.With("line", str).Debugf("Sending")
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
	queryStr := c.prefix + name + ".**"
	expandUrl, err := prepareUrl(c.graphite_web, expandEndpoint, map[string]string{"format": "json", "leavesOnly": "1", "query": queryStr})
	if err != nil {
		log.With("graphite_web", c.graphite_web).With("path", expandEndpoint).With("err", err).Warnln("Error preparing URL")
		return nil, err
	}

	// Get the list of targets
	expandResponse := ExpandResponse{}
	body, err := fetchUrl(expandUrl, ctx)
	if err != nil {
		log.With("url", expandUrl).With("err", err).Warnln("Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &expandResponse)
	if err != nil {
		log.With("url", expandUrl).With("err", err).Warnln("Error parsing expand endpoint response body")
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
		labels := metricLabelsFromPath(target, c.prefix)
		labelSet := make(model.LabelSet, len(labels))

		for _, label := range labels {
			labelSet[model.LabelName(label.Name)] = model.LabelValue(label.Value)
		}

		log.With("target", target).With("prefix", c.prefix).With("labels", labels).Debugln("Filtering target")

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
	renderUrl, err := prepareUrl(c.graphite_web, renderEndpoint, map[string]string{"format": "json", "from": from, "until": until, "target": target})
	if err != nil {
		log.With("graphite_web", c.graphite_web).With("path", renderEndpoint).With("err", err).Warnln("Error preparing URL")
		return nil, err
	}

	renderResponses := make([]RenderResponse, 0)
	body, err := fetchUrl(renderUrl, ctx)
	if err != nil {
		log.With("url", renderUrl).With("err", err).Warnln("Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &renderResponses)
	if err != nil {
		log.With("url", renderUrl).With("err", err).Warnln("Error parsing render endpoint response body")
		return nil, err
	}
	renderResponse := renderResponses[0]

	ts := &prompb.TimeSeries{}
	ts.Labels = metricLabelsFromPath(renderResponse.Target, c.prefix)
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

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	log.With("req", req).Debugf("Remote read")

	if c.graphite_web == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.read_timeout)
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

// Reloads the config.
func (c Client) ReloadConfig(configFile string) error {
	var fileConf = &config.Config{}

	if configFile != "" {
		var err error
		fileConf, err = config.LoadFile(configFile)
		if err != nil {
			log.With("err", err).Warnln("Error loading config file")
			return err
		}
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	*c.config = *fileConf
	return nil
}

// Name identifies the client as a Graphite client.
func (c Client) Name() string {
	return "graphite"
}

func (c Client) String() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.config.String()
}
