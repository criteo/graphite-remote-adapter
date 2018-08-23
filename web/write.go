package web

import (
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/criteo/graphite-remote-adapter/client"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

var (
	receivedSamples = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "received_samples_total",
			Help:      "Total number of received samples.",
		},
	)
	sentSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sent_samples_total",
			Help:      "Total number of processed samples sent to remote storage.",
		},
		[]string{"remote"},
	)
	failedSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failed_samples_total",
			Help:      "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{"remote"},
	)
	sentBatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "sent_batch_duration_seconds",
			Help:      "Duration of sample batch send calls to the remote storage.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"remote"},
	)
)

func init() {
	prometheus.MustRegister(receivedSamples)
	prometheus.MustRegister(sentSamples)
	prometheus.MustRegister(failedSamples)
	prometheus.MustRegister(sentBatchDuration)
}

func (h *Handler) write(w http.ResponseWriter, r *http.Request) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	level.Debug(h.logger).Log("request", r, "msg", "Handling /write request")
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error reading request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error decoding request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	samples := protoToSamples(&req)
	receivedSamples.Add(float64(len(samples)))

	var wg sync.WaitGroup
	for _, w := range h.writers {
		wg.Add(1)
		go func(rw client.Writer) {
			sendSamples(h.logger, rw, samples, r)
			wg.Done()
		}(w)
	}
	wg.Wait()
}

func protoToSamples(req *prompb.WriteRequest) model.Samples {
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
	return samples
}

func sendSamples(logger log.Logger, w client.Writer, samples model.Samples, r *http.Request) {
	begin := time.Now()
	err := w.Write(samples, r)
	duration := time.Since(begin).Seconds()
	if err != nil {
		level.Warn(logger).Log(
			"num_samples", len(samples), "storage", w.Name(),
			"err", err, "msg", "Error sending samples to remote storage")
		failedSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	}
	sentSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	sentBatchDuration.WithLabelValues(w.Name()).Observe(duration)
}
