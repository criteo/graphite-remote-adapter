package main

import (
	"fmt"
	"github.com/criteo/graphite-remote-adapter/client/graphite/paths"
	"github.com/criteo/graphite-remote-adapter/config"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
)

const (
	unittestHelp = `Apply a client config on imput samples in order to test this config.`
)

type unittestCmd struct {
	inputConfigFile string
}

func configureUnittestCmd(app *kingpin.Application) {
	var (
		w           = &unittestCmd{}
		unittestCmd = app.Command("unittest", unittestHelp)
	)

	unittestCmd.Flag("test.file", "Unit-test description file.").
		Required().ExistingFileVar(&w.inputConfigFile)

	unittestCmd.Action(w.Unittest)
}

func (w *unittestCmd) Unittest(ctx *kingpin.ParseContext) error {
	setupLogger()

	testCfg, err := loadUnittestConfig(w.inputConfigFile)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "error loading unit-test description file")
		return err
	}

	graCfg, err := config.LoadFile(logger, testCfg.ConfigFile)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "error loading remote-adapter configuration file")
		return err
	}

	fmt.Printf("# Testing %s\n", testCfg.ConfigFile)
	for _, testContext := range testCfg.Tests {
		fmt.Printf("## %s\n", testContext.Name)
		samples, err := makeSamples(testContext.Input)
		if err != nil {
			return err
		}

		for _, s := range samples {
			datapoints, _ := paths.ToDatapoints(s, paths.FormatCarbon, "", graCfg.Graphite.Write.Rules, graCfg.Graphite.Write.TemplateData)
			for _, dt := range datapoints {
				fmt.Print(dt)
			}
		}
	}

	return nil
}

func makeSamples(input string) ([]*model.Sample, error) {
	reader := strings.NewReader(input)
	return readSamplesFile(reader)
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

func loadUnittestConfig(filePath string) (*unittestConfig, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	cfg, err := parseUnittestConfig(content)
	if err != nil {

	}

	return cfg, nil
}

func parseUnittestConfig(content []byte) (*unittestConfig, error) {
	cfg := &unittestConfig{}
	err := yaml.Unmarshal(content, cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}