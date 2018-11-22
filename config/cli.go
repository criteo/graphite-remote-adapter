package config

import (
	"fmt"
	"os"
	"path/filepath"

	graphite "github.com/criteo/graphite-remote-adapter/client/graphite/config"
	"github.com/pkg/errors"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// ParseCommandLine parse flags and args from cli.
func ParseCommandLine() *Config {
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
		"Maximum duration before timing out remote write requests. Default is 5m").
		Default(DefaultConfig.Write.Timeout.String()).
		DurationVar(&cfg.Write.Timeout)

	a.Flag("read.timeout",
		"Maximum duration before timing out remote read requests. Default is 5m").
		Default(DefaultConfig.Read.Timeout.String()).
		DurationVar(&cfg.Read.Timeout)

	a.Flag("read.delay",
		"Duration ignoring recent samples from all remote read requests. Default is 1h").
		Default(DefaultConfig.Read.Delay.String()).
		DurationVar(&cfg.Read.Delay)

	a.Flag("read.ignore-error",
		"Avoid returning error to promtheus returning empty result instead.").
		BoolVar(&cfg.Read.IgnoreError)

	// Add logLevel flag
	a.Flag(promlogflag.LevelFlagName, promlogflag.LevelFlagHelp).
		Default("info").SetValue(&cfg.LogLevel)

	// Add graphite flag
	graphite.AddCommandLine(a, &cfg.Graphite)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}
	return cfg
}
