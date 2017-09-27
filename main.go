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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/prompb"

	"github.com/criteo/graphite-remote-adapter/client"
	"github.com/criteo/graphite-remote-adapter/graphite"
)

type config struct {
	configFile           string
	carbonAddress        string
	carbonTransport      string
	graphiteWebURL       string
	graphitePrefix       string
	remoteReadTimeout    time.Duration
	remoteWriteTimeout   time.Duration
	remoteReadDelay      time.Duration
	listenAddr           string
	telemetryPath        string
	usePathsCache        bool
	ignoreReadError      bool
	pathsCacheExpiration time.Duration
	pathsCachePurge      time.Duration
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

func reload(cfg *config, writers []client.Writer, readers []client.Reader) error {
	for _, v := range readers {
		if err := v.ReloadConfig(cfg.configFile); err != nil {
			return err
		}
	}
	for _, v := range writers {
		if err := v.ReloadConfig(cfg.configFile); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	log.Infoln("Starting graphite-remote-adapter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	cfg := parseFlags()

	http.Handle(cfg.telemetryPath, prometheus.Handler())
	writers, readers := buildClients(cfg)
	// Load the config once.
	reload(cfg, writers, readers)

	// Tooling to dynamically reload the config for each clients.
	hup := make(chan os.Signal)
	reloadCh := make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				if err := reload(cfg, writers, readers); err != nil {
					log.With("err", err).Errorln("Error reloading config")
					continue
				}
				log.Infoln("Reloaded config file")
			case rc := <-reloadCh:
				if err := reload(cfg, writers, readers); err != nil {
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
		err := serve(cfg, writers, readers)
		if err != nil {
			log.Warnln(err)
		}
	} else {
		log.Warnln("No reader nor writer, leaving")
	}
	log.Infoln("See you next time!")
}

func parseFlags() *config {
	log.Infoln("Parsing flags")
	cfg := &config{}

	flag.StringVar(&cfg.configFile, "config-file", "",
		"Graphite remote adapter configuration file name. None, if empty.",
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
	flag.DurationVar(&cfg.remoteReadDelay, "read-delay", 3600*time.Second,
		"Ignore all read requests from now to delay",
	)
	flag.StringVar(&cfg.listenAddr, "web.listen-address", ":9201", "Address to listen on for web endpoints.")
	flag.StringVar(&cfg.telemetryPath, "web.telemetry-path", "/metrics", "Address to listen on for web endpoints.")
	flag.BoolVar(&cfg.usePathsCache, "use-paths-cache", false,
		"Use a cache to store metrics paths lists.",
	)
	flag.BoolVar(&cfg.ignoreReadError, "ignore-read-error", false,
		"Ignore all read errors and return empty results instead. When enabled "+
			"prometheus will display only local points instead of returning an error.",
	)
	flag.DurationVar(&cfg.pathsCacheExpiration, "paths-cache-expiration", 3600*time.Second,
		"Expiration of items within the paths cache.",
	)
	flag.DurationVar(&cfg.pathsCachePurge, "paths-cache-purge", 2*cfg.pathsCacheExpiration,
		"Frequency of purge for expired items in the paths cache.",
	)

	flag.Parse()

	return cfg
}

func buildClients(cfg *config) ([]client.Writer, []client.Reader) {
	log.With("cfg", cfg).Infof("Building clients")
	var writers []client.Writer
	var readers []client.Reader
	if cfg.carbonAddress != "" || cfg.graphiteWebURL != "" {
		c := graphite.NewClient(
			cfg.carbonAddress, cfg.carbonTransport, cfg.remoteWriteTimeout,
			cfg.graphiteWebURL, cfg.remoteReadTimeout,
			cfg.graphitePrefix,
			cfg.remoteReadDelay, cfg.usePathsCache,
			cfg.pathsCacheExpiration, cfg.pathsCachePurge)
		if c != nil {
			writers = append(writers, c)
			readers = append(readers, c)
		}
	}
	log.With("num_writers", len(writers)).With("num_readers", len(readers)).Infof("Built clients")
	return writers, readers
}

func serve(cfg *config, writers []client.Writer, readers []client.Reader) error {
	log.Infof("Listening on %v", cfg.listenAddr)

	http.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		write(w, r, writers)
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		read(w, r, readers, cfg.ignoreReadError)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status(w, r, cfg, writers, readers)
	})

	return http.ListenAndServe(cfg.listenAddr, nil)
}

func status(w http.ResponseWriter, r *http.Request, cfg *config, writers []client.Writer, readers []client.Reader) {
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
