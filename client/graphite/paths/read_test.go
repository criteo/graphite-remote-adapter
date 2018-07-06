package paths

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestMetricLabelsFromPath(t *testing.T) {
	path := "prometheus-prefix.test.owner.team-X"
	prefix := "prometheus-prefix"
	expectedLabels := []*prompb.Label{
		&prompb.Label{Name: model.MetricNameLabel, Value: "test"},
		&prompb.Label{Name: "owner", Value: "team-X"},
	}
	actualLabels, _ := MetricLabelsFromPath(path, prefix)
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
	actualLabels, _ := MetricLabelsFromPath(path, prefix)
	require.Equal(t, expectedLabels, actualLabels)
}
