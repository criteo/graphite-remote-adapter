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

	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"golang.org/x/net/context"
)

var (
	client = &Client{
		logger: log.NewNopLogger(),
		cfg: &config.Config{
			DefaultPrefix: "prometheus-prefix.",
			Write:         config.WriteConfig{},
			Read: config.ReadConfig{
				URL: "http://fakeHost:6666",
			},
		},
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

	actualTargets, _ := client.queryToTargets(nil, query)
	if !reflect.DeepEqual(expectedTargets, actualTargets) {
		t.Errorf("Expected %s, got %s", expectedTargets, actualTargets)
	}
}

func TestInvalideQueryToTargets(t *testing.T) {
	expectedErr := fmt.Errorf("Invalide remote query: no %s label provided", model.MetricNameLabel)

	labelMatchers := []*prompb.LabelMatcher{
		&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "labelname", Value: "labelvalue"},
	}
	invalideQuery := &prompb.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	_, err := client.queryToTargets(nil, invalideQuery)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Errorf("Error from queryToTargets not returned.  Expected %v, got %v", expectedErr, err)
	}
}

func TestTargetToTimeseries(t *testing.T) {
	fetchURL = fakeFetchRenderURL
	expectedTs := &prompb.TimeSeries{
		Labels: []*prompb.Label{
			&prompb.Label{Name: model.MetricNameLabel, Value: "test"},
			&prompb.Label{Name: "owner", Value: "team-X"},
		},
		Samples: []*prompb.Sample{
			&prompb.Sample{Value: float64(18), Timestamp: int64(0)},
			&prompb.Sample{Value: float64(42), Timestamp: int64(300000)},
		},
	}

	actualTs, _ := client.targetToTimeseries(nil, "prometheus-prefix.test.owner.team-X", "0", "300")
	if !reflect.DeepEqual(expectedTs, actualTs) {
		t.Errorf("Expected %s, got %s", expectedTs, actualTs)
	}
}
