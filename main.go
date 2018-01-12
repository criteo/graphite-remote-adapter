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
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/imdario/mergo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/prompb"

	"github.com/criteo/graphite-remote-adapter/client"
	"github.com/criteo/graphite-remote-adapter/client/graphite"
	"github.com/criteo/graphite-remote-adapter/config"
)

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

func reload(cliCfg *config.Config, logger log.Logger, server *Server) (*config.Config, error) {
	cfg := &config.DefaultConfig
	// Parse config file if needed
	if cliCfg.ConfigFile != "" {
		fileCfg, err := config.LoadFile(logger, cliCfg.ConfigFile)
		if err != nil {
			level.Error(logger).Log("err", err, "msg", "Error loading config file")
			return nil, err
		}
		cfg = fileCfg
	}
	// Merge overwritting cliCfg into cfg
	if err := mergo.MergeWithOverwrite(cfg, cliCfg); err != nil {
		level.Error(logger).Log("err", err, "msg", "Error merging config file with flags")
		return nil, err
	}

	// Reload server
	if err := server.ReloadConfig(logger, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func main() {
	cliCfg := config.ParseCommandLine()
	logger := promlog.New(cliCfg.LogLevel)
	level.Info(logger).Log("msg", "Starting graphite-remote-adapter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	server := &Server{}

	// Load the config once.
	cfg, err := reload(cliCfg, logger, server)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Error first loading config")
		return
	}

	http.Handle(cfg.Web.TelemetryPath, prometheus.Handler())

	// Tooling to dynamically reload the config for each clients.
	hup := make(chan os.Signal)
	reloadCh := make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				if _, err := reload(cliCfg, logger, server); err != nil {
					level.Error(logger).Log("err", err, "msg", "Error reloading config")
					continue
				}
				level.Info(logger).Log("msg", "Reloaded config file")
			case rc := <-reloadCh:
				if _, err := reload(cliCfg, logger, server); err != nil {
					level.Error(logger).Log("err", err, "msg", "Error reloading config")
					rc <- err
				} else {
					level.Info(logger).Log("msg", "Reloaded config file")
					rc <- nil
				}
			}
		}
	}()

	http.HandleFunc("/-/reload",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				fmt.Fprintf(w, "This endpoint requires a POST request.\n")
				return
			}

			rc := make(chan error)
			reloadCh <- rc
			if err := <-rc; err != nil {
				http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
			}
		})

	if len(server.writers) != 0 || len(server.readers) != 0 {
		err := server.Serve(logger)
		if err != nil {
			level.Warn(logger).Log("err", err)
		}
	} else {
		level.Warn(logger).Log("msg", "No reader nor writer, leaving")
	}
	level.Info(logger).Log("msg", "See you next time!")
}

func buildClients(cfg *config.Config, logger log.Logger) ([]client.Writer, []client.Reader) {
	level.Info(logger).Log("cfg", cfg, "msg", "Building clients")
	var writers []client.Writer
	var readers []client.Reader
	if c := graphite.NewClient(cfg, logger); c != nil {
		writers = append(writers, c)
		readers = append(readers, c)
	}
	level.Info(logger).Log(
		"num_writers", len(writers), "num_readers", len(readers), "msg", "Built clients")
	return writers, readers
}

// Server handle http requests.
type Server struct {
	lock sync.RWMutex

	cfg *config.Config

	writers []client.Writer
	readers []client.Reader
}

// ReloadConfig reloads the config file from cli params.
func (s *Server) ReloadConfig(logger log.Logger, cfg *config.Config) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, v := range s.writers {
		v.Shutdown()
	}
	for _, v := range s.readers {
		v.Shutdown()
	}

	s.cfg = cfg
	s.writers, s.readers = buildClients(cfg, logger)

	return nil
}

// Serve handle http requests.
func (s *Server) Serve(logger log.Logger) error {
	level.Info(logger).Log("ListenAddress", s.cfg.Web.ListenAddress, "msg", "Listening")

	ihf := func(name string, f http.HandlerFunc) http.HandlerFunc {
		return prometheus.InstrumentHandlerFunc(name, func(w http.ResponseWriter, r *http.Request) {
			f(w, r)
		})
	}

	http.HandleFunc("/write", ihf("write", func(w http.ResponseWriter, r *http.Request) {
		s.Write(logger, w, r)
	}))

	http.HandleFunc("/read", ihf("read", func(w http.ResponseWriter, r *http.Request) {
		s.Read(logger, w, r)
	}))

	http.HandleFunc("/", ihf("status", func(w http.ResponseWriter, r *http.Request) {
		s.Status(w, r)
	}))

	return http.ListenAndServe(s.cfg.Web.ListenAddress, nil)
}

// Status generate an html status page.
func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.status(w, r)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Graphite Remote Adapter</title>

    <!-- Bootstrap -->
    <link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.7/css/bootstrap.min.css" integrity="sha384-BVYiiSIFeK1dGmJRAkycuHAHRg32OmUcww7on3RYdg4Va+PmSTsz/K68vbdEjh4u" crossorigin="anonymous">
    <link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.7/css/bootstrap-theme.min.css" integrity="sha384-rHyoN1iRsVXV4nD0JutlnGaslCJuC7uwjduW9SVrLvRYooPp2bWYgmgJQIXwl/Sp" crossorigin="anonymous">


  </head>
  <body>
    <div class="container" role="main">
    <h1>Graphite Remote Adapter</h1>
`)

	fmt.Fprintf(w, "graphite-remote-adapter %s<br/>", version.Info())
	fmt.Fprintf(w, "Build context %s<br/>", version.BuildContext())

	fmt.Fprintf(w, "Flags:<br/><pre>%s</pre>", html.EscapeString(spew.Sdump(s.cfg)))

	fmt.Fprintf(w, "Writers:<br/><dl>")
	for _, v := range s.writers {
		spew.Fprintf(w, "<dt>%s</dt><dd><pre>%s</pre></dd>",
			v.Name(), html.EscapeString(spew.Sdump(v)))
	}
	fmt.Fprintf(w, "</dl>")
	fmt.Fprintf(w, "Readers:<br/><dl>")
	for _, v := range s.readers {
		spew.Fprintf(w, "<dt>%s</dt><dd><pre>%s</pre></dd>",
			v.Name(), html.EscapeString(spew.Sdump(v)))
	}
	fmt.Fprintf(w, "</dl>")

	fmt.Fprintf(w, `
    </div>
    <script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.7/js/bootstrap.min.js" integrity="sha384-Tc5IQib027qvyjSMfHjOMaLkfuWVxZxUPnCJA7l2mCWNIpG9mGCD8wGNIcPD7Txa" crossorigin="anonymous"></script>
  </body>
</html>
`)
}

func (s *Server) Write(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.write(logger, w, r)
}

func (s *Server) write(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	level.Debug(logger).Log("request", r, "msg", "Handling /write request")
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error reading request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error decoding request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	samples := protoToSamples(&req)
	receivedSamples.Add(float64(len(samples)))

	var wg sync.WaitGroup
	for _, w := range s.writers {
		wg.Add(1)
		go func(rw client.Writer) {
			sendSamples(logger, rw, samples)
			wg.Done()
		}(w)
	}
	wg.Wait()
}

func (s *Server) Read(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.read(logger, w, r)
}

func (s *Server) read(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	level.Debug(logger).Log("request", r, "msg", "Handling /read request")
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error reading request body")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error decoding request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.ReadRequest
	if err = proto.Unmarshal(reqBuf, &req); err != nil {
		level.Warn(logger).Log("err", err, "msg", "Error unmarshalling protobuf")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Support reading from more than one reader and merging the results.
	if len(s.readers) != 1 {
		http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(s.readers)), http.StatusInternalServerError)
		return
	}
	reader := s.readers[0]

	var resp *prompb.ReadResponse
	resp, err = reader.Read(&req)
	if err != nil {
		level.Warn(logger).Log(
			"query", req, "storage", reader.Name(),
			"err", err, "msg", "Error executing query")
		if s.cfg.Read.IgnoreError == false {
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

func sendSamples(logger log.Logger, w client.Writer, samples model.Samples) {
	begin := time.Now()
	err := w.Write(samples)
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
