package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"golang.org/x/net/context/ctxhttp"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const (
	mockWriteHelp = `Send a write request to the remote-adapter as a Prometheus would do in compressed protobuf.`
)

type mockWriteCmd struct {
	inputMetricsFile string
	remoteAdapterURL *url.URL
}

func configureMockWriteCmd(app *kingpin.Application) {
	var (
		w            = &mockWriteCmd{}
		mockWriteCmd = app.Command("mock-write", mockWriteHelp)
	)
	addMetricsFileFlag(mockWriteCmd, &w.inputMetricsFile)

	mockWriteCmd.Flag("remote-adapter.url", "Set a default remote-adapter url to use for each request.").
		Required().URLVar(&w.remoteAdapterURL)

	mockWriteCmd.Action(w.MockWrite)
}

func (w *mockWriteCmd) MockWrite(ctx *kingpin.ParseContext) error {
	setupLogger()
	samples, err := loadSamplesFile(w.inputMetricsFile)
	if err != nil {
		return err
	}

	req := toWriteRequest(samples)

	err = sendWriteRequestAsProm(req, w.remoteAdapterURL)
	if err != nil {
		return err
	}
	return nil
}

// toWriteRequest converts an array of samples into a WriteRequest proto.
func toWriteRequest(samples []*model.Sample) *prompb.WriteRequest {
	req := &prompb.WriteRequest{
		Timeseries: make([]*prompb.TimeSeries, 0, len(samples)),
	}

	for _, s := range samples {
		ts := prompb.TimeSeries{
			Labels: metricToLabelProtos(s.Metric),
			Samples: []prompb.Sample{
				{
					Value:     float64(s.Value),
					Timestamp: int64(s.Timestamp),
				},
			},
		}
		req.Timeseries = append(req.Timeseries, &ts)
	}

	return req
}

// metricToLabelProtos builds a []*prompb.Label from a model.Metric
func metricToLabelProtos(metric model.Metric) []*prompb.Label {
	labels := make([]*prompb.Label, 0, len(metric))
	for k, v := range metric {
		labels = append(labels, &prompb.Label{
			Name:  string(k),
			Value: string(v),
		})
	}
	sort.Slice(labels, func(i int, j int) bool {
		return labels[i].Name < labels[j].Name
	})
	return labels
}

func sendWriteRequestAsProm(req *prompb.WriteRequest, remoteAdapterURL *url.URL) error {
	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	compressed := snappy.Encode(nil, data)

	client := &http.Client{}
	u, err := url.Parse("/write")
	if err != nil {
		return err
	}
	writeURL := remoteAdapterURL.ResolveReference(u)
	httpReq, err := http.NewRequest("POST", writeURL.String(), bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	httpResp, err := ctxhttp.Do(ctx, client, httpReq)
	if err != nil {
		return err
	}
	level.Info(logger).Log("status", httpResp.StatusCode)

	b, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	os.Stdout.Write(b)

	return nil
}
