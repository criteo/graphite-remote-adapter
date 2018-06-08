package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"golang.org/x/net/context/ctxhttp"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const (
	mockWriteHelp = `Send a request to the remote-adapter as a Prometheus would do.

The remote-adapter tool will read an input file in Prometheus exposition text format;
translate it in WriteRequest in compressed protobuf format; and send it to
the remote-adapter url on its /write endpoint.
`
)

type mockWriteCmd struct {
	inputMetricsFile string
}

func configureMockWriteCmd(app *kingpin.Application) {
	var (
		w            = &mockWriteCmd{}
		mockWriteCmd = app.Command("mock-write", mockWriteHelp).PreAction(requireRemoteAdapterURL)
	)
	mockWriteCmd.Flag("input-metrics-file", "Filename containing input metrics in prometheus export format.").
		Short('i').StringVar(&w.inputMetricsFile)
	mockWriteCmd.Action(w.MockWrite)
}

func (w *mockWriteCmd) MockWrite(ctx *kingpin.ParseContext) error {
	samples, err := loadSamplesFile(w.inputMetricsFile)
	if err != nil {
		return err
	}

	req := toWriteRequest(samples)

	err = sendWriteRequestAsProm(req)
	if err != nil {
		return err
	}
	return nil
}

func loadSamplesFile(filename string) ([]*model.Sample, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	dec := &expfmt.SampleDecoder{
		Dec: expfmt.NewDecoder(file, expfmt.FmtText),
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.Now(),
		},
	}

	var all model.Vector
	for {
		var smpls model.Vector
		err := dec.Decode(&smpls)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		all = append(all, smpls...)
	}

	return all, nil
}

// toWriteRequest converts an array of samples into a WriteRequest proto.
func toWriteRequest(samples []*model.Sample) *prompb.WriteRequest {
	req := &prompb.WriteRequest{
		Timeseries: make([]*prompb.TimeSeries, 0, len(samples)),
	}

	for _, s := range samples {
		ts := prompb.TimeSeries{
			Labels: metricToLabelProtos(s.Metric),
			Samples: []*prompb.Sample{
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

func sendWriteRequestAsProm(req *prompb.WriteRequest) error {
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

	// TODO use proper logger
	fmt.Println(httpResp.StatusCode, ": ", httpResp.Body)
	return nil
}
