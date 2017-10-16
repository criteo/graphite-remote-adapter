package config

import (
	"fmt"
	"os"
	"path/filepath"

	graphite "github.com/criteo/graphite-remote-adapter/graphite/config"
	"github.com/pkg/errors"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func ParseCommandLine() *Config {
	log.Infoln("Parsing command line")
	cfg := &Config{}

	a := kingpin.New(filepath.Base(os.Args[0]), "The Graphite remote adapter")

	a.Version(version.Print("graphite-remote-adapter"))

	a.HelpFlag.Short('h')

	a.Flag("config.file", "Graphite-remote-adapter configuration file path.").
		StringVar(&cfg.ConfigFile)

	a.Flag("web.listen-address", "Address to listen on for UI and telemtry.").
		StringVar(&cfg.Web.ListenAddress)

	a.Flag("web.telemetry-path", "Path to listen for telemtry.").
		StringVar(&cfg.Web.TelemetryPath)

	a.Flag("write.timeout",
		"Maximum duration before timing out remote write requests.").
		DurationVar(&cfg.Write.Timeout)

	a.Flag("read.timeout",
		"Maximum duration before timing out remote read requests.").
		DurationVar(&cfg.Read.Timeout)

	a.Flag("read.delay",
		"Duration ignoring recent samples from all remote read requests.").
		DurationVar(&cfg.Read.Delay)

	a.Flag("read.ignore-error",
		"Avoid returning error to promtheus returning empty result instead.").
		BoolVar(&cfg.Read.IgnoreError)

	graphite.AddCommandLine(a, &cfg.Graphite)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}
	return cfg
}
