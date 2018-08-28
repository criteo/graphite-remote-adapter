package paths

import (
	"fmt"
	"sort"
	"strings"

	graphite_tmpl "github.com/criteo/graphite-remote-adapter/client/graphite/template"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

func MetricLabelsFromTags(tags map[string]string, prefix string) ([]*prompb.Label, error) {
	// It translates Graphite tags directly into label and values.
	var labels []*prompb.Label
	var names []string

	// Sort tags  to have a deterministic order, better for tests.
	for k := range tags {
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

func MetricLabelsFromPath(path string, prefix string) ([]*prompb.Label, error) {
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
		labels = append(labels, &prompb.Label{Name: graphite_tmpl.Unescape(nodes[i]), Value: graphite_tmpl.Unescape(nodes[i+1])})
	}
	return labels, nil
}
