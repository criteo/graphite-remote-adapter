package web

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/prompb"
)

var (
	readSamples = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "read_samples_total",
			Help:      "Total number of samples read from remote storage.",
		},
		[]string{"prefix", "remote"},
	)
	failedReads = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failed_reads_total",
			Help:      "Total number of reads which failed on the remote storage.",
		},
		[]string{"prefix", "remote"},
	)
)

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	level.Debug(h.logger).Log("request", r, "msg", "Handling /read request")
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

	var req prompb.ReadRequest
	if err = proto.Unmarshal(reqBuf, &req); err != nil {
		level.Warn(h.logger).Log("err", err, "msg", "Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Support reading from more than one reader and merging the results.
	if len(h.readers) != 1 {
		http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(h.readers)), http.StatusInternalServerError)
		return
	}
	reader := h.readers[0]
	prefix := h.cfg.Graphite.StoragePrefixFromRequest(r)

	var resp *prompb.ReadResponse
	resp, err = reader.Read(&req, r)
	if err != nil {
		level.Warn(h.logger).Log(
			"query", req, "storage", reader.Name(),
			"err", err, "msg", "Error executing query")
		failedReads.WithLabelValues(prefix, reader.Target()).Inc()
		if h.cfg.Read.IgnoreError == false {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if resp == nil {
		resp = &prompb.ReadResponse{
			Results: []*prompb.QueryResult{
				{Timeseries: make([]*prompb.TimeSeries, 0, 0)},
			},
		}
	} else {
		readSamples.WithLabelValues(prefix, reader.Target()).Add(float64(resp.Size()))
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")

	compressed = snappy.Encode(nil, data)
	if _, err := w.Write(compressed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
