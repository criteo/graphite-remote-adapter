// Copyright 2015 The Prometheus Authors
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
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	pmetric "github.com/prometheus/prometheus/storage/metric"

	"golang.org/x/net/context"
	"strings"
)

func (c *Client) queryToTargets(ctx context.Context, query *prompb.Query) ([]string, error) {
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
	expandURL, err := prepareURL(c.cfg.Read.URL, expandEndpoint, map[string]string{"format": "json", "leavesOnly": "1", "query": queryStr})
	if err != nil {
		level.Warn(c.logger).Log(
			"graphite_web", c.cfg.Read.URL, "path", expandEndpoint,
			"err", err, "msg", "Error preparing URL")
		return nil, err
	}

	// Get the list of targets
	expandResponse := ExpandResponse{}
	body, err := fetchURL(ctx, c.logger, expandURL)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", expandURL, "err", err, "msg", "Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &expandResponse)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", expandURL, "err", err,
			"msg", "Error parsing expand endpoint response body")
		return nil, err
	}

	targets, err := c.filterTargets(query, expandResponse.Results)
	return targets, err
}

func (c *Client) queryToTargetsWithTags(ctx context.Context, query *prompb.Query) ([]string, error) {
	tagSet := []string{}

	for _, m := range query.Matchers {
		var name string
		var value string
		if m.Name == model.MetricNameLabel {
			name = "name"
			value = c.cfg.DefaultPrefix + m.Value
		} else {
			name = m.Name
			value = m.Value
		}

		switch m.Type {
		case prompb.LabelMatcher_EQ:
			tagSet = append(tagSet, "\""+name+"="+value+"\"")
		case prompb.LabelMatcher_NEQ:
			tagSet = append(tagSet, "\""+name+"!="+value+"\"")
		case prompb.LabelMatcher_RE:
			tagSet = append(tagSet, "\""+name+"=~^("+value+")$\"")
		case prompb.LabelMatcher_NRE:
			tagSet = append(tagSet, "\""+name+"!=~^("+value+")$\"")
		default:
			return nil, fmt.Errorf("unknown match type %v", m.Type)
		}
	}

	targets := []string{"seriesByTag(" + strings.Join(tagSet, ",") + ")"}
	return targets, nil
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
			"labels", labelSet, "msg", "Filtering target")

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

func (c *Client) targetToTimeseries(ctx context.Context, target string, from string, until string) ([]*prompb.TimeSeries, error) {
	renderURL, err := prepareURL(c.cfg.Read.URL, renderEndpoint, map[string]string{"format": "json", "from": from, "until": until, "target": target})
	if err != nil {
		level.Warn(c.logger).Log(
			"graphite_web", c.cfg.Read.URL, "path", renderEndpoint,
			"err", err, "msg", "Error preparing URL")
		return nil, err
	}

	renderResponses := make([]RenderResponse, 0)
	body, err := fetchURL(ctx, c.logger, renderURL)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", renderURL, "err", err, "ctx", ctx, "msg", "Error fetching URL")
		return nil, err
	}

	err = json.Unmarshal(body, &renderResponses)
	if err != nil {
		level.Warn(c.logger).Log(
			"url", renderURL, "err", err,
			"msg", "Error parsing render endpoint response body")
		return nil, err
	}

	ret := make([]*prompb.TimeSeries, len(renderResponses))
	for i, renderResponse := range renderResponses {
		ts := &prompb.TimeSeries{}

		if c.cfg.EnableTags {
			ts.Labels, err = metricLabelsFromTags(renderResponse.Tags, c.cfg.DefaultPrefix)
		} else {
			ts.Labels, err = metricLabelsFromPath(renderResponse.Target, c.cfg.DefaultPrefix)
		}

		if err != nil {
			level.Warn(c.logger).Log(
				"path", renderResponse.Target, "prefix", c.cfg.DefaultPrefix, "err", err)
			return nil, err
		}
		for _, datapoint := range renderResponse.Datapoints {
			timstampMs := datapoint.Timestamp * 1000
			if datapoint.Value == nil {
				continue
			}
			ts.Samples = append(ts.Samples, &prompb.Sample{Value: *datapoint.Value, Timestamp: timstampMs})
		}
		ret[i] = ts
	}
	return ret, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) handleReadQuery(ctx context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
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
	fromStr := strconv.Itoa(from)
	untilStr := strconv.Itoa(until)

	targets := []string{}
	var err error

	if c.cfg.EnableTags {
		targets, err = c.queryToTargetsWithTags(ctx, query)
	} else {
		// If we don't have tags we try to emulate then with normal paths.
		targets, err = c.queryToTargets(ctx, query)
	}
	if err != nil {
		return nil, err
	}

	level.Debug(c.logger).Log(
		"targets", targets, "from", fromStr, "until", untilStr, "msg", "Fetching data")
	c.fetchData(ctx, queryResult, targets, fromStr, untilStr)
	return queryResult, nil

}

func (c *Client) fetchData(ctx context.Context, queryResult *prompb.QueryResult, targets []string, fromStr string, untilStr string) {
	input := make(chan string, len(targets))
	output := make(chan *prompb.TimeSeries, len(targets)+1)

	wg := sync.WaitGroup{}

	// TODO: Send multiple targets per query, Graphite supports that.
	// Start only a few workers to avoid killing graphite.
	for i := 0; i < maxFetchWorkers; i++ {
		wg.Add(1)

		go func(fromStr string, untilStr string, ctx context.Context) {
			defer wg.Done()

			for target := range input {
				// We simply ignore errors here as it is better to return "some" data
				// than nothing.
				ts, err := c.targetToTimeseries(ctx, target, fromStr, untilStr)
				if err != nil {
					level.Warn(c.logger).Log("target", target, "err", err, "msg", "Error fetching and parsing target datapoints")
				} else {
					level.Debug(c.logger).Log("reading responses")
					for _, t := range ts {
						output <- t
					}
				}
			}
		}(fromStr, untilStr, ctx)
	}

	// Feed the input.
	for _, target := range targets {
		input <- target
	}
	close(input)

	// Close the output as soon as all jobs are done.
	go func() {
		wg.Wait()
		output <- nil
		close(output)
	}()

	// Read output until channel is closed.
	for {
		done := false
		select {
		case ts := <-output:
			if ts != nil {
				queryResult.Timeseries = append(queryResult.Timeseries, ts)
			} else {
				// A nil result means that we are done.
				done = true
			}
		}
		if done {
			break
		}
	}
}

// Read implements the client.Reader interface.
func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	level.Debug(c.logger).Log("req", req, "msg", "Remote read")

	if c.cfg.Read.URL == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.readTimeout)
	defer cancel()

	resp := &prompb.ReadResponse{}
	for _, query := range req.Queries {
		queryResult, err := c.handleReadQuery(ctx, query)
		if err != nil {
			return nil, err
		}
		resp.Results = append(resp.Results, queryResult)
	}
	return resp, nil
}
