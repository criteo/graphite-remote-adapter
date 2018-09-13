package main

import (
	"fmt"
	"github.com/andreyvit/diff"
	"github.com/criteo/graphite-remote-adapter/client/graphite/paths"
	"github.com/criteo/graphite-remote-adapter/config"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/sergi/go-diff/diffmatchpatch"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	unittestHelp = `Apply a client config on imput samples in order to test this config.`
)

type unittestCmd struct {
	inputTestFile string
}

func configureUnittestCmd(app *kingpin.Application) {
	var (
		w           = &unittestCmd{}
		unittestCmd = app.Command("unittest", unittestHelp)
	)

	unittestCmd.Flag("test.file", "Unit-test description file.").
		Required().ExistingFileVar(&w.inputTestFile)

	unittestCmd.Action(w.Unittest)
}

func (w *unittestCmd) Unittest(ctx *kingpin.ParseContext) error {
	setupLogger()

	testCfg, err := loadUnittestConfig(w.inputTestFile)
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
	hasDiffs := false
	for _, testContext := range testCfg.Tests {
		fmt.Printf("## %s\n", testContext.Name)
		output, err := makeSortedOutput(testContext, graCfg)
		if err != nil {
			level.Error(logger).Log("err", err, "msg", fmt.Sprintf("failed to generate output for test case %s", testContext.Name))
			return err
		}
		outputDiff := makeDiff(testContext.Output, output)
		if len(outputDiff) > 0 {
			hasDiffs = true
			fmt.Println("Unexpected output:")
			fmt.Println(strings.Join(outputDiff, "\n"))
		} else {
			fmt.Println("OK")
		}
	}

	if hasDiffs {
		os.Exit(-1)
	}

	return nil
}

func makeDiff(expected string, actual string) []string {
	expected = strings.Trim(expected, "\n")
	actual = strings.Trim(actual, "\n")
	dmp := diffmatchpatch.New()
	diffResult := dmp.DiffMain(expected, actual, false)
	for _, r := range diffResult {
		if r.Type != diffmatchpatch.DiffEqual {
			// Generate patch-style diff
			return diff.LineDiffAsLines(expected, actual)
		}
	}

	return nil
}

func makeSortedOutput(testContext *testConfig, graCfg *config.Config) (string, error) {
	samples, err := makeSamples(testContext.Input)
	if err != nil {
		return "", err
	}

	var outputPaths []string
	for _, s := range samples {
		datapoints, _ := paths.ToDatapoints(s, paths.FormatCarbon, "", graCfg.Graphite.Write.Rules, graCfg.Graphite.Write.TemplateData)
		for _, dt := range datapoints {
			outputPaths = append(outputPaths, dt)
		}
	}
	sort.Strings(outputPaths)
	return strings.Join(outputPaths, ""), nil
}

func makeSamples(input string) ([]*model.Sample, error) {
	reader := strings.NewReader(input)
	return readSamples(reader)
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
		return nil, err
	}

	// Make config file path relative to test file
	testFileDir := filepath.Dir(filePath)
	configFilePath := filepath.Join(testFileDir, cfg.ConfigFile)
	cfg.ConfigFile, err = filepath.Abs(configFilePath)
	if err != nil {
		return nil, err
	}

	// Sanitize test definition
	for _, test := range cfg.Tests {
		output := strings.Split(test.Output, "\n")
		sort.Strings(output)
		test.Output = strings.Join(output, "\n")
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
