package paths

import (
	"github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/prometheus/common/model"
)

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
