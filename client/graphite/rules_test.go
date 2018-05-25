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
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
)

var (
	metric = model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-X",
		"many_chars":          "abc!ABC:012-3!45ö67~89./(){},=.\"\\",
	}
	metricY = model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-Y",
		"many_chars":          "abc!ABC:012-3!45ö67~89./(){},=.\"\\",
	}

	testConfigStr = `
write:
  template_data:
    shared: data.foo
  rules:
  - match:
      owner: team-X
    match_re:
      testlabel: ^test:.*$
    template: 'tmpl_1.{{.shared | escape}}.{{.labels.owner}}'
    continue: true
  - match:
      owner: team-X
      testlabel2:   test:value2
    template: 'tmpl_2.{{.labels.owner}}.{{.shared}}'
    continue: false
  - match:
      owner: team-Y
    template: 'tmpl_3.{{.labels.owner}}.{{.shared}}'
    continue: false
  - match:
      owner: team-Z
    continue: false`

	testConfig = loadTestConfig(testConfigStr)
)

func loadTestConfig(s string) *config.Config {
	cfg := &config.Config{}
	if err := yaml.Unmarshal([]byte(string(s)), cfg); err != nil {
		return nil
	}
	return cfg
}

func TestDefaultPathsFromMetric(t *testing.T) {
	expected := "prefix." +
		"test:metric" +
		".many_chars.abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\" +
		".owner.team-X" +
		".testlabel.test:value"
	actual, err := pathsFromMetric(metric, FormatCarbon, "prefix.", nil, nil)
	require.Equal(t, expected, actual[0])
	require.Empty(t, err)

	expected = "prefix." +
		"test:metric" +
		";many_chars=abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\" +
		";owner=team-X" +
		";testlabel=test:value"

	actual, err = pathsFromMetric(metric, FormatCarbonTags, "prefix.", nil, nil)
	require.Equal(t, expected, actual[0])
	require.Empty(t, err)

	expected = "prefix." +
		"test:metric{" +
		"many_chars=\"abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\\"" +
		",owner=\"team-X\"" +
		",testlabel=\"test:value\"" +
		"}"
	actual, err = pathsFromMetric(metric, FormatCarbonOpenMetrics, "prefix.", nil, nil)
	require.Equal(t, expected, actual[0])
	require.Empty(t, err)
}

func TestUnmatchedMetricPathsFromMetric(t *testing.T) {
	unmatchedMetric := model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-K",
		"testlabel2":          "test:value2",
	}
	expected := make([]string, 0)
	expected = append(expected, "prefix."+
		"test:metric"+
		".owner.team-K"+
		".testlabel.test:value"+
		".testlabel2.test:value2")
	actual, err := pathsFromMetric(unmatchedMetric, FormatCarbon, "prefix.", testConfig.Write.Rules, testConfig.Write.TemplateData)
	require.Equal(t, expected, actual)
	require.Empty(t, err)
}

func TestTemplatedPathsFromMetric(t *testing.T) {
	expected := make([]string, 0)
	expected = append(expected, "tmpl_3.team-Y.data.foo")
	actual, err := pathsFromMetric(metricY, FormatCarbon, "", testConfig.Write.Rules, testConfig.Write.TemplateData)
	require.Equal(t, expected, actual)
	require.Empty(t, err)
}

func TestTemplatedPathsFromMetricWithDefault(t *testing.T) {
	expected := make([]string, 0)
	expected = append(expected, "tmpl_1.data%2Efoo.team-X")
	expected = append(expected, "prefix."+
		"test:metric"+
		".many_chars.abc!ABC:012-3!45%C3%B667~89%2E%2F\\(\\)\\{\\}\\,%3D%2E\\\"\\\\"+
		".owner.team-X"+
		".testlabel.test:value")
	actual, err := pathsFromMetric(metric, FormatCarbon, "prefix.", testConfig.Write.Rules, testConfig.Write.TemplateData)
	require.Equal(t, expected, actual)
	require.Empty(t, err)
}

func TestMultiTemplatedPathsFromMetric(t *testing.T) {
	multiMatchMetric := model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-X",
		"testlabel2":          "test:value2",
	}
	expected := make([]string, 0)
	expected = append(expected, "tmpl_1.data%2Efoo.team-X")
	expected = append(expected, "tmpl_2.team-X.data.foo")
	actual, err := pathsFromMetric(multiMatchMetric, FormatCarbon, "prefix.", testConfig.Write.Rules, testConfig.Write.TemplateData)
	require.Equal(t, expected, actual)
	require.Empty(t, err)
}

func TestSkipedTemplatedPathsFromMetric(t *testing.T) {
	skipedMetric := model.Metric{
		model.MetricNameLabel: "test:metric",
		"testlabel":           "test:value",
		"owner":               "team-Z",
		"testlabel2":          "test:value2",
	}
	t.Log(testConfig.Write.Rules[2])
	actual, err := pathsFromMetric(skipedMetric, FormatCarbon, "", testConfig.Write.Rules, testConfig.Write.TemplateData)
	require.Empty(t, actual)
	require.Empty(t, err)
}

func TestMetricLabelsFromPath(t *testing.T) {
	path := "prometheus-prefix.test.owner.team-X"
	prefix := "prometheus-prefix"
	expectedLabels := []*prompb.Label{
		&prompb.Label{Name: model.MetricNameLabel, Value: "test"},
		&prompb.Label{Name: "owner", Value: "team-X"},
	}
	actualLabels, _ := metricLabelsFromPath(path, prefix)
	require.Equal(t, expectedLabels, actualLabels)
}
func TestMetricLabelsFromSpecialPath(t *testing.T) {
	path := "prometheus-prefix.test.owner.team-Y.interface.Hu0%2F0%2F1%2F3%2E99"
	prefix := "prometheus-prefix"
	expectedLabels := []*prompb.Label{
		&prompb.Label{Name: model.MetricNameLabel, Value: "test"},
		&prompb.Label{Name: "owner", Value: "team-Y"},
		&prompb.Label{Name: "interface", Value: "Hu0/0/1/3.99"},
	}
	actualLabels, _ := metricLabelsFromPath(path, prefix)
	require.Equal(t, expectedLabels, actualLabels)
}

func TestReplaceNilLabelTemplatedPathsFromMetric(t *testing.T) {
	testConfigNilLabelStr := `
write:
  rules:
  - match_re:
      testlabel: test:value
    template: 'test.{{ replace .labels.doesnotexist " " "_" }}'
    continue: false`

	testConfigNilLabel := loadTestConfig(testConfigNilLabelStr)

	t.Log(testConfigNilLabel.Write.Rules[0])
	actual, err := pathsFromMetric(metric, FormatCarbon, "", testConfigNilLabel.Write.Rules, testConfigNilLabel.Write.TemplateData)
	require.Empty(t, actual)
	require.Error(t, err)
}
