package paths

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
	graphite_tmpl "github.com/criteo/graphite-remote-adapter/client/graphite/template"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/common/model"
)

// ToDatapoints builds points from samples.
func ToDatapoints(s *model.Sample, format Format, prefix string, rules []*config.Rule, templateData map[string]interface{}) ([]string, error) {
	t := float64(s.Timestamp.UnixNano()) / 1e9
	v := float64(s.Value)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errors.New("invalid sample value")
	}

	paths, err := pathsFromMetric(s.Metric, format, prefix, rules, templateData)
	if err != nil {
		return nil, err
	}

	datapoints := []string{}
	for _, path := range paths {
		datapoints = append(datapoints, fmt.Sprintf("%s %f %.0f\n", path, v, t))
	}
	return datapoints, nil
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
	buffer.WriteString(graphite_tmpl.Escape(string(m[model.MetricNameLabel])))

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
		v := graphite_tmpl.Escape(string(m[l]))

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
