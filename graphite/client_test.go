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

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/remote"

	"golang.org/x/net/context"
)

var (
	client = &Client{
		graphite_web: "http://fakeHost:6666",
		prefix:       "prometheus-prefix.",
	}
)

func TestPrepareUrl(t *testing.T) {
	expectedUrl := "https://guest:guest@greathost:83232/my/path?q=query&toto=lulu"

	u, _ := prepareUrl("https://guest:guest@greathost:83232", "/my/path", map[string]string{"q": "query", "toto": "lulu"})
	actualUrl := u.String()

	if actualUrl != expectedUrl {
		t.Errorf("Expected %s, got %s", expectedUrl, actualUrl)
	}
}

func fakeFetchExpandUrl(u *url.URL, ctx context.Context) ([]byte, error) {
	var body bytes.Buffer
	if u.String() == "http://fakeHost:6666/metrics/expand?format=json&leavesOnly=1&query=prometheus-prefix.test.%2A%2A" {
		body.WriteString("{\"results\": [\"prometheus-prefix.test.owner.team-X\", \"prometheus-prefix.test.owner.team-Y\"]}")
	}
	return body.Bytes(), nil
}

func fakeFetchRenderUrl(u *url.URL, ctx context.Context) ([]byte, error) {
	var body bytes.Buffer
	if u.String() == "http://fakeHost:6666/render/?format=json&from=0&target=prometheus-prefix.test.owner.team-X&until=300" {
		body.WriteString("[{\"target\": \"prometheus-prefix.test.owner.team-X\", \"datapoints\": [[18,0], [42,300]]}]")
	}
	return body.Bytes(), nil
}

func TestQueryToTargets(t *testing.T) {
	fetchUrl = fakeFetchExpandUrl
	expectedTargets := []string{"prometheus-prefix.test.owner.team-X", "prometheus-prefix.test.owner.team-Y"}

	labelMatchers := []*remote.LabelMatcher{&remote.LabelMatcher{Type: remote.MatchType_EQUAL, Name: model.MetricNameLabel, Value: "test"}}
	query := &remote.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	actualTargets, _ := client.queryToTargets(query, nil)
	if !reflect.DeepEqual(expectedTargets, actualTargets) {
		t.Errorf("Expected %s, got %s", expectedTargets, actualTargets)
	}
}

func TestInvalideQueryToTargets(t *testing.T) {
	expectedErr := fmt.Errorf("Invalide remote query: no %s label provided", model.MetricNameLabel)

	labelMatchers := []*remote.LabelMatcher{&remote.LabelMatcher{Type: remote.MatchType_EQUAL, Name: "labelname", Value: "labelvalue"}}
	invalideQuery := &remote.Query{
		StartTimestampMs: int64(0),
		EndTimestampMs:   int64(300),
		Matchers:         labelMatchers,
	}

	_, err := client.queryToTargets(invalideQuery, nil)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Errorf("Error from queryToTargets not returned.  Expected %v, got %v", expectedErr, err)
	}
}

func TestTargetToTimeseries(t *testing.T) {
	fetchUrl = fakeFetchRenderUrl
	expectedTs := &remote.TimeSeries{
		Labels:  []*remote.LabelPair{&remote.LabelPair{Name: model.MetricNameLabel, Value: "test"}, &remote.LabelPair{Name: "owner", Value: "team-X"}},
		Samples: []*remote.Sample{&remote.Sample{Value: float64(18), TimestampMs: int64(0)}, &remote.Sample{Value: float64(42), TimestampMs: int64(300000)}},
	}

	actualTs, _ := client.targetToTimeseries("prometheus-prefix.test.owner.team-X", "0", "300", nil)
	if !reflect.DeepEqual(expectedTs, actualTs) {
		t.Errorf("Expected %s, got %s", expectedTs, actualTs)
	}
}
