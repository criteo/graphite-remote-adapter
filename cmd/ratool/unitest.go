package main

import (
	"fmt"
	"github.com/criteo/graphite-remote-adapter/client/graphite/paths"
	"github.com/criteo/graphite-remote-adapter/config"
	"github.com/go-kit/kit/log/level"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

const (
	unittestHelp = `Apply a client config on imput samples in order to test this config.`
)

type unittestCmd struct {
	inputMetricsFile string
	configFile       string
	clientType       string
}

func configureUnittestCmd(app *kingpin.Application) {
	var (
		w           = &unittestCmd{}
		unittestCmd = app.Command("unittest", unittestHelp)
	)

	addMetricsFileFlag(unittestCmd, &w.inputMetricsFile)

	unittestCmd.Flag("config.file", "Graphite-remote-adapter configuration file path.").
		Required().ExistingFileVar(&w.configFile)

	unittestCmd.Flag("client", "Graphite-remote-adapter client to use.").
		Default("graphite").EnumVar(&w.clientType, "graphite")

	unittestCmd.Action(w.Unittest)
}

func (w *unittestCmd) Unittest(ctx *kingpin.ParseContext) error {
	setupLogger()
	fileCfg, err := config.LoadFile(logger, w.configFile)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Error loading config file")
		return err
	}

	samples, err := loadSamplesFile(w.inputMetricsFile)
	if err != nil {
		return err
	}

	if w.clientType == "graphite" {
		for _, s := range samples {
			datapoints, _ := paths.ToDatapoints(s, paths.FormatCarbon, "", fileCfg.Graphite.Write.Rules, fileCfg.Graphite.Write.TemplateData)
			for _, dt := range datapoints {
				fmt.Print(dt)
			}
		}
	}

	return nil
}

type unittestConfig struct {
	ConfigFile string        `yaml:"config_file"`
	Tests      []*testConfig `yaml:"tests"`
}

type testConfig struct {
	Name   string `yaml:"name"`
	Input  string `yaml:"input"`
	Output string `yaml:"output"`
}

func parseUnittestConfig(content []byte) (*unittestConfig, error) {
	cfg := &unittestConfig{}
	err := yaml.Unmarshal(content, cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
