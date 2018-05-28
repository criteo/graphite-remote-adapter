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
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/criteo/graphite-remote-adapter/utils"

	"github.com/patrickmn/go-cache"
)

type Format int

const (
	FormatCarbon            Format = 1
	FormatCarbonTags               = 2
	FormatCarbonOpenMetrics        = 3
)

var (
	pathsCache        *cache.Cache
	pathsCacheEnabled = false
)

func initPathsCache(pathsCacheTTL time.Duration, pathsCachePurgeInterval time.Duration) {
	pathsCache = cache.New(pathsCacheTTL, pathsCachePurgeInterval)
	pathsCacheEnabled = true
}

func loadContext(templateData map[string]interface{}, m model.Metric) map[string]interface{} {
	ctx := make(map[string]interface{})
	for k, v := range templateData {
		ctx[k] = v
	}
	labels := make(map[string]string)
	for ln, lv := range m {
		labels[string(ln)] = string(lv)
	}
	ctx["labels"] = labels
	return ctx
}

func match(m model.Metric, match config.LabelSet, matchRE config.LabelSetRE) bool {
	for ln, lv := range match {
		if m[ln] != lv {
			return false
		}
	}
	for ln, r := range matchRE {
		if !r.MatchString(string(m[ln])) {
			return false
		}
	}
	return true
}

func pathsFromMetric(m model.Metric, format Format, prefix string, rules []*config.Rule, templateData map[string]interface{}) ([]string, error) {
	var err error
	if pathsCacheEnabled {
		cachedPaths, cached := pathsCache.Get(m.Fingerprint().String())
		if cached {
			return cachedPaths.([]string), nil
		}
	}
	paths, stop, err := templatedPaths(m, rules, templateData)
	// if it doesn't match any rule, use default path
	if !stop {
		paths = append(paths, defaultPath(m, format, prefix))
	}
	if pathsCacheEnabled {
		pathsCache.Set(m.Fingerprint().String(), paths, cache.DefaultExpiration)
	}
	return paths, err
}

func templatedPaths(m model.Metric, rules []*config.Rule, templateData map[string]interface{}) ([]string, bool, error) {
	var paths []string
	var stop = false
	var err error
	for _, rule := range rules {
		match := match(m, rule.Match, rule.MatchRE)
		if !match {
			continue
		}
		// We have a rule to silence this metric
		if rule.Continue == false && (rule.Tmpl == config.Template{}) {
			return nil, true, nil
		}

		context := loadContext(templateData, m)
		stop = !rule.Continue
		var path bytes.Buffer
		err = rule.Tmpl.Execute(&path, context)
		if err != nil {
			// We had an error processing the template so we break the loop
			break
		}
		paths = append(paths, path.String())

		if rule.Continue == false {
			break
		}
	}
	return paths, stop, err
}

func defaultPath(m model.Metric, format Format, prefix string) string {
	var buffer bytes.Buffer
	var lbuffer bytes.Buffer

	buffer.WriteString(prefix)
	buffer.WriteString(utils.Escape(string(m[model.MetricNameLabel])))

	// We want to sort the labels.
	labels := make(model.LabelNames, 0, len(m))
	for l := range m {
		labels = append(labels, l)
	}
	sort.Sort(labels)

	first := true
	for _, l := range labels {
		if l == model.MetricNameLabel || len(l) == 0 {
			continue
		}

		k := string(l)
		v := utils.Escape(string(m[l]))

		if format == FormatCarbonOpenMetrics {
			// https://github.com/RichiH/OpenMetrics/blob/master/metric_exposition_format.md
			if !first {
				lbuffer.WriteString(",")
			}
			lbuffer.WriteString(fmt.Sprintf("%s=\"%s\"", k, v))
		} else if format == FormatCarbonTags {
			// See http://graphite.readthedocs.io/en/latest/tags.html
			lbuffer.WriteString(fmt.Sprintf(";%s=%s", k, v))
		} else {
			// For each label, in order, add ".<label>.<value>".
			// Since we use '.' instead of '=' to separate label and values
			// it means that we can't have an '.' in the metric name. Fortunately
			// this is prohibited in prometheus metrics.
			lbuffer.WriteString(fmt.Sprintf(".%s.%s", k, v))
		}
		first = false
	}

	if lbuffer.Len() > 0 {
		if format == FormatCarbonOpenMetrics {
			buffer.WriteRune('{')
			buffer.Write(lbuffer.Bytes())
			buffer.WriteRune('}')
		} else {
			buffer.Write(lbuffer.Bytes())
		}
	}
	return buffer.String()
}

func metricLabelsFromTags(tags Tags, prefix string) ([]*prompb.Label, error) {
	// It translates Graphite tags directly into label and values.
	var labels []*prompb.Label
	var names []string

	// Sort tags  to have a deterministic order, better for tests.
	for k, _ := range tags {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, k := range names {
		v := tags[k]
		if k == "name" {
			v = strings.TrimPrefix(v, prefix)
			labels = append(labels, &prompb.Label{Name: model.MetricNameLabel, Value: v})
		} else {
			labels = append(labels, &prompb.Label{Name: k, Value: v})
		}
	}

	return labels, nil
}

func metricLabelsFromPath(path string, prefix string) ([]*prompb.Label, error) {
	// It uses the "default" write format to read back (See defaultPath function)
	// <prefix.><__name__.>[<labelName>.<labelValue>. for each label in alphabetic order]
	var labels []*prompb.Label
	cleanedPath := strings.TrimPrefix(path, prefix)
	cleanedPath = strings.Trim(cleanedPath, ".")
	nodes := strings.Split(cleanedPath, ".")
	labels = append(labels, &prompb.Label{Name: model.MetricNameLabel, Value: nodes[0]})
	if len(nodes[1:])%2 != 0 {
		err := fmt.Errorf("Unable to parse labels from path: odd number of nodes in path")
		return nil, err
	}
	for i := 1; i < len(nodes); i += 2 {
		labels = append(labels, &prompb.Label{Name: utils.Unescape(nodes[i]), Value: utils.Unescape(nodes[i+1])})
	}
	return labels, nil
}
