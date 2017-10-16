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
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/imdario/mergo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/prompb"

	"github.com/criteo/graphite-remote-adapter/client"
	"github.com/criteo/graphite-remote-adapter/config"
	"github.com/criteo/graphite-remote-adapter/graphite"
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

func reload(cliCfg *config.Config, writers []client.Writer, readers []client.Reader, server server) (*config.Config, error) {
	cfg := &config.DefaultConfig
	// Parse config file if needed
	if cliCfg.ConfigFile != "" {
		fileCfg, err := config.LoadFile(cliCfg.ConfigFile)
		if err != nil {
			log.With("err", err).Warnln("Error loading config file")
			return nil, err
		}
		cfg = fileCfg
	}
	// Merge overwritting cliCfg into cfg
	if err := mergo.MergeWithOverwrite(cfg, cliCfg); err != nil {
		log.With("err", err).Warnln("Error loading config file")
		return nil, err
	}

	// Reload clients
	for _, r := range readers {
		if err := r.ReloadConfig(cfg); err != nil {
			return nil, err
		}
	}
	for _, w := range writers {
		if err := w.ReloadConfig(cfg); err != nil {
			return nil, err
		}
	}
	if err := server.ReloadConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	log.Infoln("Starting graphite-remote-adapter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	cliCfg := config.ParseCommandLine()

	s := server{}
	writers, readers := buildClients(cliCfg)

	// Load the config once.
	cfg, err := reload(cliCfg, writers, readers, s)
	if err != nil {
		log.With("err", err).Warnln("Error first loading config")
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
				if _, err := reload(cliCfg, writers, readers, s); err != nil {
					log.With("err", err).Errorln("Error reloading config")
					continue
				}
				log.Infoln("Reloaded config file")
			case rc := <-reloadCh:
				if _, err := reload(cliCfg, writers, readers, s); err != nil {
					log.With("err", err).Errorln("Error reloading config")
					rc <- err
				} else {
					log.Infoln("Reloaded config file")
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

	if len(writers) != 0 || len(readers) != 0 {
		err := s.serve(writers, readers)
		if err != nil {
			log.Warnln(err)
		}
	} else {
		log.Warnln("No reader nor writer, leaving")
	}
	log.Infoln("See you next time!")
}

func buildClients(cfg *config.Config) ([]client.Writer, []client.Reader) {
	log.With("cfg", cfg).Infof("Building clients")
	var writers []client.Writer
	var readers []client.Reader
	if c := graphite.NewClient(cfg); c != nil {
		writers = append(writers, c)
		readers = append(readers, c)
	}
	log.With("num_writers", len(writers)).With("num_readers", len(readers)).Infof("Built clients")
	return writers, readers
}

type server struct {
	cfg *config.Config
}

// Reloads the config.
func (s server) ReloadConfig(cfg *config.Config) error {
	s.cfg = cfg
	return nil
}

func (s *server) serve(writers []client.Writer, readers []client.Reader) error {
	log.Infof("Listening on %v", s.cfg.Web.ListenAddress)

	http.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		write(w, r, writers)
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		read(w, r, readers, s.cfg.Read.IgnoreError)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status(w, r, s.cfg, writers, readers)
	})

	return http.ListenAndServe(s.cfg.Web.ListenAddress, nil)
}

func status(w http.ResponseWriter, r *http.Request, cfg *config.Config, writers []client.Writer, readers []client.Reader) {
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

	fmt.Fprintf(w, "Flags:<br/><pre>%s</pre>", html.EscapeString(spew.Sdump(cfg)))

	fmt.Fprintf(w, "Writers:<br/><dl>")
	for _, v := range writers {
		spew.Fprintf(w, "<dt>%s</dt><dd><pre>%s</pre></dd>",
			v.Name(), html.EscapeString(spew.Sdump(v)))
	}
	fmt.Fprintf(w, "</dl>")
	fmt.Fprintf(w, "Readers:<br/><dl>")
	for _, v := range readers {
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

func write(w http.ResponseWriter, r *http.Request, writers []client.Writer) {
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

	var req prompb.WriteRequest
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
		go func(rw client.Writer) {
			sendSamples(rw, samples)
			wg.Done()
		}(w)
	}
	wg.Wait()
}

func read(w http.ResponseWriter, r *http.Request, readers []client.Reader, ignore_read_error bool) {
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

	var req prompb.ReadRequest
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

	var resp *prompb.ReadResponse
	resp, err = reader.Read(&req)
	if err != nil {
		log.With("query", req).With("storage", reader.Name()).With("err", err).Warnf("Error executing query")
		if ignore_read_error == false {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			resp = &prompb.ReadResponse{
				Results: []*prompb.QueryResult{
					{Timeseries: make([]*prompb.TimeSeries, 0, 0)},
				},
			}
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

func sendSamples(w client.Writer, samples model.Samples) {
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
