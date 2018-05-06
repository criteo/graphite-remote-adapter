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
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/criteo/graphite-remote-adapter/ui"
	"github.com/criteo/graphite-remote-adapter/utils"

	assetfs "github.com/elazarl/go-bindata-assetfs"
)

// Constants for instrumentation.
const namespace = "remote_adapter"

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

	http.HandleFunc("/-/healthy", ihf("healthy", func(w http.ResponseWriter, r *http.Request) {
		s.Healthy(w, r)
	}))

	staticFs := http.FileServer(
		&assetfs.AssetFS{Asset: ui.Asset, AssetDir: ui.AssetDir, AssetInfo: ui.AssetInfo, Prefix: ""})
	http.Handle("/static/", prometheus.InstrumentHandler("static", staticFs))

	http.HandleFunc("/", ihf("status", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, "Page not found... Sorry :(", http.StatusNotFound)
			return
		}
		s.Status(w, r)
	}))

	return http.ListenAndServe(s.cfg.Web.ListenAddress, nil)
}

func (s *Server) getTemplate(name string) (string, error) {
	baseTmpl, err := ui.Asset("templates/_base.html")
	if err != nil {
		return "", fmt.Errorf("error reading base template: %s", err)
	}
	pageTmpl, err := ui.Asset(filepath.Join("templates", name))
	if err != nil {
		return "", fmt.Errorf("error reading page template %s: %s", name, err)
	}
	return string(baseTmpl) + string(pageTmpl), nil
}

func (s *Server) executeTemplate(w http.ResponseWriter, name string, data interface{}) ([]byte, error) {
	text, err := s.getTemplate(name)
	if err != nil {
		return nil, err
	}

	tmpl := template.New(name).Funcs(template.FuncMap(utils.TmplFuncMap))
	tmpl, err = tmpl.Parse(text)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, data)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// Healthy generate an html healthy page.
func (s *Server) Healthy(w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.healthy(w, r)
}

func (s *Server) healthy(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// Status generate an html status page.
func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	s.status(w, r)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	status := struct {
		VersionInfo         string
		VersionBuildContext string
		Cfg                 string
		Readers             map[string]string
		Writers             map[string]string
	}{
		VersionInfo:         version.Info(),
		VersionBuildContext: version.BuildContext(),
		Cfg:                 html.EscapeString(spew.Sdump(s.cfg)),
		Readers:             map[string]string{},
		Writers:             map[string]string{},
	}
	for _, r := range s.readers {
		status.Readers[r.Name()] = html.EscapeString(spew.Sdump(r))
	}
	for _, w := range s.writers {
		status.Writers[w.Name()] = html.EscapeString(spew.Sdump(w))
	}

	bytes, err := s.executeTemplate(w, "status.html", status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
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
			sendSamples(logger, rw, samples, r)
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
	resp, err = reader.Read(&req, r)
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
