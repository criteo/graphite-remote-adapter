// Copyright 2015 The Prometheus Authors
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
	"testing"

	"github.com/prometheus/common/model"

	"github.com/criteo/graphite-remote-adapter/graphite/config"
)

var (
	metric = model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-X",
		"many_chars":          "abc!ABC:012-3!45ö67~89./(){},=.\"\\",
	}

	testConfigStr = `
template_data:
    shared: data

rules:
  - match:
      owner: team-X
    match_re:
      testlabel: ^test:.*$
    template: 'tmpl_1.{{.shared}}.{{.labels.owner}}'
    continue: true
  - match:
      owner: team-X
      testlabel2:   test:value2
    template: 'tmpl_2.{{.labels.owner}}.{{.shared}}'
    continue: true
  - match:
      owner: team-Z
    continue: false`

	testConfig, _ = config.Load(string(testConfigStr))
)

func TestDefaultPathsFromMetric(t *testing.T) {
	expected := make([]string, 0)
	expected = append(expected, "prefix."+
		"test:metric"+
		".many_chars.abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\"+
		".owner.team-X"+
		".testlabel.test:value")
	actual := pathsFromMetric(metric, "prefix.", nil, nil)
	if len(actual) != 1 || expected[0] != actual[0] {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestTemplatedPathsFromMetric(t *testing.T) {
	expected := make([]string, 0)
	expected = append(expected, "tmpl_1.data.team-X")
	actual := pathsFromMetric(metric, "", testConfig.Rules, testConfig.Template_data)
	if len(actual) != 1 || expected[0] != actual[0] {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestMultiTemplatedPathsFromMetric(t *testing.T) {
	multiMatchMetric := model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-X",
		"testlabel2":          "test:value2",
	}
	expected := make([]string, 0)
	expected = append(expected, "tmpl_1.data.team-X")
	expected = append(expected, "tmpl_2.team-X.data")
	actual := pathsFromMetric(multiMatchMetric, "", testConfig.Rules, testConfig.Template_data)
	if len(actual) != 2 || expected[0] != actual[0] || expected[1] != actual[1] {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestSkipedTemplatedPathsFromMetric(t *testing.T) {
	skipedMetric := model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-Z",
		"testlabel2":          "test:value2",
	}
	t.Log(testConfig.Rules[2])
	expected := make([]string, 0)
	actual := pathsFromMetric(skipedMetric, "", testConfig.Rules, testConfig.Template_data)
	if len(actual) != 0 {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}
