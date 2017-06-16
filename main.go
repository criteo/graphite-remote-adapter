// Copyright 2017 The Prometheus Authors
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

// The main package for the Prometheus server executable.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/storage/remote"

	"github.com/criteo/graphite-remote-adapter/graphite"
)

type config struct {
	configFile         string
	carbonAddress      string
	carbonTransport    string
	graphiteWebURL     string
	graphitePrefix     string
	remoteReadTimeout  time.Duration
	remoteWriteTimeout time.Duration
	listenAddr         string
	telemetryPath      string
}

var (
	receivedSamples = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "received_samples_total",
			Help: "Total number of received samples.",
		},
	)
	sentSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sent_samples_total",
			Help: "Total number of processed samples sent to remote storage.",
		},
		[]string{"remote"},
	)
	failedSamples = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "failed_samples_total",
			Help: "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{"remote"},
	)
	sentBatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sent_batch_duration_seconds",
			Help:    "Duration of sample batch send calls to the remote storage.",
			Buckets: prometheus.DefBuckets,
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

func main() {
	log.Infoln("Starting graphite-remote-adapter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	cfg := parseFlags()
	http.Handle(cfg.telemetryPath, prometheus.Handler())

	writers, readers := buildClients(cfg)
	serve(cfg.listenAddr, writers, readers)
	log.Infoln("See you next time!")
}

func parseFlags() *config {
	log.Infoln("Parsing flags")
	cfg := &config{}

	flag.StringVar(&cfg.configFile, "config-file", "graphite-remote-adapter.yml",
		"Graphite remote adapter configuration file name.",
	)
	flag.StringVar(&cfg.carbonAddress, "carbon-address", "",
		"The host:port of the Graphite server to send samples to. None, if empty.",
	)
	flag.StringVar(&cfg.carbonTransport, "carbon-transport", "tcp",
		"Transport protocol to use to communicate with Graphite. 'tcp', if empty.",
	)
	flag.StringVar(&cfg.graphiteWebURL, "graphite-url", "",
		"The URL of the remote Graphite Web server to send samples to. None, if empty.",
	)
	flag.StringVar(&cfg.graphitePrefix, "graphite-prefix", "",
		"The prefix to prepend to all metrics exported to Graphite. None, if empty.",
	)
	flag.DurationVar(&cfg.remoteWriteTimeout, "write-timeout", 30*time.Second,
		"The timeout to use when writing samples to the remote storage.",
	)
	flag.DurationVar(&cfg.remoteReadTimeout, "read-timeout", 30*time.Second,
		"The timeout to use when reading samples to the remote storage.",
	)
	flag.StringVar(&cfg.listenAddr, "web.listen-address", ":9201", "Address to listen on for web endpoints.")
	flag.StringVar(&cfg.telemetryPath, "web.telemetry-path", "/metrics", "Address to listen on for web endpoints.")

	flag.Parse()

	return cfg
}

type writer interface {
	Write(samples model.Samples) error
	Name() string
}

type reader interface {
	Read(req *remote.ReadRequest) (*remote.ReadResponse, error)
	Name() string
}

func buildClients(cfg *config) ([]writer, []reader) {
	log.With("cfg", cfg).Infof("Building clients")
	var writers []writer
	var readers []reader
	if cfg.carbonAddress != "" || cfg.graphiteWebURL != "" {
		c := graphite.NewClient(
			cfg.carbonAddress, cfg.carbonTransport, cfg.remoteWriteTimeout,
			cfg.graphiteWebURL, cfg.remoteReadTimeout,
			cfg.graphitePrefix, cfg.configFile)
		writers = append(writers, c)
		readers = append(readers, c)
	}
	log.With("num_writers", len(writers)).With("num_readers", len(readers)).Infof("Built clients")
	return writers, readers
}

func serve(addr string, writers []writer, readers []reader) error {
	log.Infof("Listening on %v", addr)

	http.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		write(w, r, writers)
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		read(w, r, readers)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status(w, r, writers, readers)
	})

	return http.ListenAndServe(addr, nil)
}

func status(w http.ResponseWriter, r *http.Request, writers []writer, readers []reader) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "graphite-remote-adapter %s<br/>", version.Info())
	fmt.Fprintf(w, "Build context %s<br/>", version.BuildContext())

	fmt.Fprintf(w, "Writers:<br/><dl>")
	for _, v := range writers {
		fmt.Fprintf(w, "<dt>%s</dt><dd><pre>%s</pre></dd>", v.Name(), v)
	}
	fmt.Fprintf(w, "</dl>")
	fmt.Fprintf(w, "Readers:<br/><dl>")
	for _, v := range readers {
		fmt.Fprintf(w, "<dt>%s</dt><dd><pre>%s</pre></dd>", v.Name(), v)
	}
	fmt.Fprintf(w, "</dl>")
}

func write(w http.ResponseWriter, r *http.Request, writers []writer) {
	log.With("request", r).Debugln("Handling /write request")
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.With("err", err).Warnln("Error reading request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		log.With("err", err).Warnln("Error decoding request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req remote.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		log.With("err", err).Warnln("Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	samples := protoToSamples(&req)
	receivedSamples.Add(float64(len(samples)))

	var wg sync.WaitGroup
	for _, w := range writers {
		wg.Add(1)
		go func(rw writer) {
			sendSamples(rw, samples)
			wg.Done()
		}(w)
	}
	wg.Wait()
}

func read(w http.ResponseWriter, r *http.Request, readers []reader) {
	log.With("request", r).Debugln("Handling /read request")
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.With("err", err).Warnln("Error reading request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		log.With("err", err).Warnln("Error decoding request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req remote.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		log.With("err", err).Warnln("Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Support reading from more than one reader and merging the results.
	if len(readers) != 1 {
		http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(readers)), http.StatusInternalServerError)
		return
	}
	reader := readers[0]

	var resp *remote.ReadResponse
	resp, err = reader.Read(&req)
	if err != nil {
		log.With("query", req).With("storage", reader.Name()).With("err", err).Warnf("Error executing query")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

func protoToSamples(req *remote.WriteRequest) model.Samples {
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
				Timestamp: model.Time(s.TimestampMs),
			})
		}
	}
	return samples
}

func sendSamples(w writer, samples model.Samples) {
	begin := time.Now()
	err := w.Write(samples)
	duration := time.Since(begin).Seconds()
	if err != nil {
		log.With("num_samples", len(samples)).With("storage", w.Name()).With("err", err).Warnf("Error sending samples to remote storage")
		failedSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	}
	sentSamples.WithLabelValues(w.Name()).Add(float64(len(samples)))
	sentBatchDuration.WithLabelValues(w.Name()).Observe(duration)
}
