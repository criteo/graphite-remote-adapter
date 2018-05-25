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
	"bytes"
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"golang.org/x/net/context"
)

var (
	expectedLabels = []*prompb.Label{
		&prompb.Label{Name: model.MetricNameLabel, Value: "test"},
		&prompb.Label{Name: "owner", Value: "team-X"},
	}
	expectedSamples = []*prompb.Sample{
		&prompb.Sample{Value: float64(18), Timestamp: int64(0)},
		&prompb.Sample{Value: float64(42), Timestamp: int64(300000)},
	}
)

func fakeFetchExpandURL(ctx context.Context, l log.Logger, u *url.URL) ([]byte, error) {
	var body bytes.Buffer
	if u.String() == "http://fakeHost:6666/metrics/expand?format=json&leavesOnly=1&query=prometheus-prefix.test.%2A%2A" {
		body.WriteString("{\"results\": [\"prometheus-prefix.test.owner.team-X\", \"prometheus-prefix.test.owner.team-Y\"]}")
	}
	return body.Bytes(), nil
}

func fakeFetchRenderURL(ctx context.Context, l log.Logger, u *url.URL) ([]byte, error) {
	var body bytes.Buffer
	if u.String() == "http://fakeHost:6666/render/?format=json&from=0&target=prometheus-prefix.test.owner.team-X&until=300" {
		body.WriteString("[{\"target\": \"prometheus-prefix.test.owner.team-X\", \"datapoints\": [[18,0], [42,300]]}]")
	} else if u.String() == "http://fakeHost:6666/render/?format=json&from=0&target=seriesByTag%28%22name%3Dprometheus-prefix.test%22%2C%22owner%3Dteam-x%22%29&until=300" {
		body.WriteString("[")
		body.WriteString("{\"target\": \"prometheus-prefix.test\", \"tags\": {\"owner\": \"team-X\", \"name\": \"prometheus-prefix.test\"}, \"datapoints\": [[18,0], [42,300]]},")
		body.WriteString("{\"target\": \"prometheus-prefix.test\", \"tags\": {\"owner\": \"team-X\", \"name\": \"prometheus-prefix.test\", \"foo\": \"bar\"}, \"datapoints\": [[18,0], [42,300]]}")
		body.WriteString("]")
	}
	return body.Bytes(), nil
}

func TestQueryToTargets(t *testing.T) {
	fetchURL = fakeFetchExpandURL
	expectedTargets := []string{"prometheus-prefix.test.owner.team-X", "prometheus-prefix.test.owner.team-Y"}

	labelMatchers := []*prompb.LabelMatcher{
		// Query a specific metric.
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: model.MetricNameLabel, Value: "test"},
		// Validate that we can match labels.
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_RE, Name: "owner", Value: "team.*"},
		// Also check that we are not equal to a fake label.
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_NEQ, Name: "invalid.", Value: "fake"},
	}
	query := &prompb.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	actualTargets, _ := testClient.queryToTargets(nil, query, testClient.cfg.DefaultPrefix)
	if !reflect.DeepEqual(expectedTargets, actualTargets) {
		t.Errorf("Expected %s, got %s", expectedTargets, actualTargets)
	}
}

func TestInvalidQueryToTargets(t *testing.T) {
	expectedErr := fmt.Errorf("Invalid remote query: no %s label provided", model.MetricNameLabel)

	labelMatchers := []*prompb.LabelMatcher{
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "labelname", Value: "labelvalue"},
	}
	invalidQuery := &prompb.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	_, err := testClient.queryToTargets(nil, invalidQuery, testClient.cfg.DefaultPrefix)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Errorf("Error from queryToTargets not returned.  Expected %v, got %v", expectedErr, err)
	}
}

func TestTargetToTimeseries(t *testing.T) {
	fetchURL = fakeFetchRenderURL
	expectedTs := &prompb.TimeSeries{
		Labels:  expectedLabels,
		Samples: expectedSamples,
	}

	actualTs, err := testClient.targetToTimeseries(nil, "prometheus-prefix.test.owner.team-X", "0", "300", testClient.cfg.DefaultPrefix)
	if !reflect.DeepEqual(err, nil) {
		t.Errorf("Expected no err, got %s", err)
	}
	if !reflect.DeepEqual(expectedTs, actualTs[0]) {
		t.Errorf("Expected %s, got %s", expectedTs, actualTs[0])
	}
}

func TestQueryTargetsWithTags(t *testing.T) {
	fetchURL = fakeFetchRenderURL

	labelMatchers := []*prompb.LabelMatcher{
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: model.MetricNameLabel, Value: "test"},
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "owner", Value: "team-x"},
	}
	query := &prompb.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	expectedTargets := []string{
		"seriesByTag(\"name=prometheus-prefix.test\",\"owner=team-x\")",
	}

	expectedTs := []*prompb.TimeSeries{
		&prompb.TimeSeries{
			Labels:  expectedLabels,
			Samples: expectedSamples,
		},
		&prompb.TimeSeries{
			Labels: []*prompb.Label{
				&prompb.Label{Name: "foo", Value: "bar"},
				expectedLabels[0],
				expectedLabels[1],
			},
			Samples: expectedSamples,
		},
	}

	testClient.cfg.EnableTags = true
	targets, err := testClient.queryToTargetsWithTags(nil, query, testClient.cfg.DefaultPrefix)
	if err != nil {
		t.Errorf("Unexpected err: %s", err)
	}
	if !reflect.DeepEqual(expectedTargets, targets) {
		t.Errorf("Expected %s, got %s", expectedTargets, targets)
	}

	actualTs, err := testClient.targetToTimeseries(nil, targets[0], "0", "300", testClient.cfg.DefaultPrefix)
	testClient.cfg.EnableTags = false
	if err != nil {
		t.Errorf("Unexpected err: %s", err)
	}
	if !reflect.DeepEqual(expectedTs, actualTs) {
		t.Errorf("Expected %s, got %s", expectedTs, actualTs)
	}
}
