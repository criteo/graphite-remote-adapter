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

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"github.com/criteo/graphite-remote-adapter/graphite/config"
	"github.com/criteo/graphite-remote-adapter/utils"

	"github.com/patrickmn/go-cache"
)

var (
	paths_cache         *cache.Cache
	paths_cache_enabled = false
)

func initPathsCache(pathsCacheExpiration time.Duration, pathsCachePurge time.Duration) {
	paths_cache = cache.New(pathsCacheExpiration, pathsCachePurge)
	paths_cache_enabled = true
	log.Infof("Paths cache initialized with %s expiration duration and %s cleanup interval", pathsCacheExpiration.String(), pathsCachePurge.String())
}

func loadContext(template_data map[string]interface{}, m model.Metric) map[string]interface{} {
	ctx := make(map[string]interface{})
	for k, v := range template_data {
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

func pathsFromMetric(m model.Metric, prefix string, rules []*config.Rule, template_data map[string]interface{}) []string {
	if paths_cache_enabled {
		cached_assertion, cached := paths_cache.Get(m.Fingerprint().String())
		if cached {
			return cached_assertion.([]string)
		}
	}
	paths, skipped := templatedPaths(m, rules, template_data)
	// if it doesn't match any rule, use default path
	if len(paths) == 0 && !skipped {
		paths = append(paths, defaultPath(m, prefix))
	}
	if paths_cache_enabled {
		paths_cache.Set(m.Fingerprint().String(), paths, cache.DefaultExpiration)
	}
	return paths
}

func templatedPaths(m model.Metric, rules []*config.Rule, template_data map[string]interface{}) ([]string, bool) {
	var paths []string
	for _, rule := range rules {
		match := match(m, rule.Match, rule.MatchRE)
		if !match {
			continue
		}
		// We have a rule to silence this metric
		if rule.Continue == false && (rule.Tmpl == config.Template{}) {
			return nil, true
		}

		context := loadContext(template_data, m)
		var path bytes.Buffer
		rule.Tmpl.Execute(&path, context)
		paths = append(paths, path.String())

		if rule.Continue == false {
			break
		}
	}
	return paths, false
}

func defaultPath(m model.Metric, prefix string) string {
	var buffer bytes.Buffer

	buffer.WriteString(prefix)
	buffer.WriteString(utils.Escape(string(m[model.MetricNameLabel])))

	// We want to sort the labels.
	labels := make(model.LabelNames, 0, len(m))
	for l := range m {
		labels = append(labels, l)
	}
	sort.Sort(labels)

	// For each label, in order, add ".<label>.<value>".
	for _, l := range labels {
		v := m[l]

		if l == model.MetricNameLabel || len(l) == 0 {
			continue
		}
		// Since we use '.' instead of '=' to separate label and values
		// it means that we can't have an '.' in the metric name. Fortunately
		// this is prohibited in prometheus metrics.
		buffer.WriteString(fmt.Sprintf(
			".%s.%s", string(l), utils.Escape(string(v))))
	}
	return buffer.String()
}

func metricLabelsFromPath(path string, prefix string) []*prompb.Label {
	// It uses the "default" write format to read back (See defaultPath function)
	// <prefix.><__name__.>[<labelName>.<labelValue>. for each label in alphabetic order]
	var labels []*prompb.Label
	cleanedPath := strings.TrimPrefix(path, prefix)
	cleanedPath = strings.Trim(cleanedPath, ".")
	nodes := strings.Split(cleanedPath, ".")
	labels = append(labels, &prompb.Label{Name: model.MetricNameLabel, Value: nodes[0]})
	if len(nodes[1:])%2 != 0 {
		log.With("path", path).With("prefix", prefix).Warnln("Unable to parse labels from path: odd number of nodes in path")
		return labels
	}
	for i := 1; i < len(nodes); i += 2 {
		labels = append(labels, &prompb.Label{Name: nodes[i], Value: nodes[i+1]})
	}
	return labels
}
