package web

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/criteo/graphite-remote-adapter/client"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

var (
	receivedSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "received_samples_total",
			Help:      "Total number of received samples.",
		},
		[]string{"prefix"},
	)
	sentSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sent_samples_total",
			Help:      "Total number of processed samples sent to remote storage.",
		},
		[]string{"prefix", "remote"},
	)
	failedSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failed_samples_total",
			Help:      "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{"prefix", "remote"},
	)
	sentBatchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "sent_batch_duration_seconds",
			Help:      "Duration of sample batch send calls to the remote storage.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"remote"},
	)
)

func (h *Handler) write(w http.ResponseWriter, r *http.Request) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	level.Debug(h.logger).Log("request", r, "msg", "Handling /write request")

	// As default we expected snappy encoded protobuf.
	// But for simulation prupose we also accept json.
	dryRun := false
	if ct := r.Header.Get("Content-Type"); ct == "application/json" {
		dryRun = true
	}

	// Parse samples from request.
	var samples model.Samples
	var err error
	if dryRun {
		samples, err = h.parseFakeWriteRequest(w, r)
	} else {
		samples, err = h.parseWriteRequest(w, r)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prefix := h.cfg.Graphite.StoragePrefixFromRequest(r)

	receivedSamples.WithLabelValues(prefix).Add(float64(len(samples)))

	// Execute write on each writer clients.
	var wg sync.WaitGroup
	writeResponse := make(map[string]string)
	for _, writer := range h.writers {
		wg.Add(1)
		go func(client client.Writer) {
			msgBytes, err := h.instrumentedWriteSamples(client, samples, r, dryRun)
			if err != nil {
				failedSamples.WithLabelValues(prefix, client.Target()).Add(float64(len(samples)))
				writeResponse[client.Name()] = err.Error()
			} else {
				sentSamples.WithLabelValues(prefix, client.Target()).Add(float64(len(samples)))
				writeResponse[client.Name()] = string(msgBytes)
			}
			wg.Done()
		}(writer)
	}
	wg.Wait()

	// Write response body.
	data, err := json.Marshal(writeResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func (h *Handler) parseFakeWriteRequest(w http.ResponseWriter, r *http.Request) (model.Samples, error) {
	decoder := json.NewDecoder(r.Body)
	var samples []*model.Sample
	err := decoder.Decode(&samples)
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func (h *Handler) parseWriteRequest(w http.ResponseWriter, r *http.Request) (model.Samples, error) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error reading request body")
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error decoding request body")
		return nil, err
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error unmarshalling protobuf")
		return nil, err
	}

	var samples model.Samples
	for _, ts := range req.Timeseries {
		metric := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}

		for _, s := range ts.Samples {
			samples = append(samples, &model.Sample{
				Metric:    metric,
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.Timestamp),
			})
		}
	}
	return samples, nil
}

func (h *Handler) instrumentedWriteSamples(
	w client.Writer, samples model.Samples, r *http.Request, dryRun bool) ([]byte, error) {

	begin := time.Now()
	msgBytes, err := w.Write(samples, r, dryRun)
	duration := time.Since(begin).Seconds()
	if err != nil {
		level.Warn(h.logger).Log(
			"num_samples", len(samples), "storage", w.Name(),
			"err", err, "msg", "Error sending samples to remote storage")
		return nil, err
	}
	sentBatchDuration.WithLabelValues(w.Target()).Observe(duration)
	return msgBytes, nil
}
