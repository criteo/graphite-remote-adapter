package main

import (
	"io"
	"os"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func setupLogger() {
	if logger == nil {
		logger = promlog.New(logLevel)
	}
}

func addMetricsFileFlag(command *kingpin.CmdClause, target *string) {
	command.Flag("metrics.file", "Filename containing input metrics in prometheus export format.").
		Required().ExistingFileVar(target)
}

func loadSamplesFile(filename string) ([]*model.Sample, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return readSamples(file)
}

func readSamples(reader io.Reader) ([]*model.Sample, error) {
	dec := &expfmt.SampleDecoder{
		Dec: expfmt.NewDecoder(reader, expfmt.FmtText),
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.Now(),
		},
	}

	var all model.Vector
	for {
		var smpls model.Vector
		err := dec.Decode(&smpls)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		all = append(all, smpls...)
	}

	return all, nil
}
