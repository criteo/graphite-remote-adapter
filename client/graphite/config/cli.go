package config

import (
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// AddCommandLine setup Graphite specific cli args and flags.
func AddCommandLine(app *kingpin.Application, cfg *Config) {
	app.Flag("graphite.default-prefix",
		"The prefix to prepend to all metrics exported to Graphite.").
		StringVar(&cfg.DefaultPrefix)

	app.Flag("graphite.read.url",
		"The URL of the remote Graphite Web server to send samples to.").
		StringVar(&cfg.Read.URL)

	app.Flag("graphite.write.carbon-address",
		"The host:port of the Graphite server to send samples to.").
		StringVar(&cfg.Write.CarbonAddress)

	app.Flag("graphite.write.carbon-transport",
		"Transport protocol to use to communicate with Graphite.").
		StringVar(&cfg.Write.CarbonTransport)

	app.Flag("graphite.write.enable-paths-cache",
		"Enables a cache to graphite paths lists for written metrics.").
		BoolVar(&cfg.Write.EnablePathsCache)

	app.Flag("graphite.write.paths-cache-ttl",
		"Duration TTL of items within the paths cache.").
		DurationVar(&cfg.Write.PathsCacheTTL)

	app.Flag("graphite.write.paths-cache-purge-interval",
		"Duration between purges for expired items in the paths cache.").
		DurationVar(&cfg.Write.PathsCachePurgeInterval)
}
