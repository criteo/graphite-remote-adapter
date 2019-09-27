package main

import (
	"os"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const (
	helpRoot = `Tool to interact with a remote-adapter and its configuration.`
)

var (
	defaultLogLevel promlog.AllowedLevel
	logger          log.Logger
)

func main() {
	app := kingpin.New("ratool", helpRoot).DefaultEnvars()

	// Add logLevel flag
	app.Flag(promlogflag.LevelFlagName, promlogflag.LevelFlagHelp).
		Default("info").SetValue(&defaultLogLevel)

	configureMockWriteCmd(app)
	configureUnittestCmd(app)

	app.GetFlag("help").Short('h')
	kingpin.MustParse(app.Parse(os.Args[1:]))
}
