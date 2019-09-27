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
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/imdario/mergo"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"

	"github.com/criteo/graphite-remote-adapter/config"
	"github.com/criteo/graphite-remote-adapter/web"
)

func reload(cliCfg *config.Config, logger log.Logger) (*config.Config, error) {
	cfg := &config.DefaultConfig
	// Parse config file if needed
	if cliCfg.ConfigFile != "" {
		fileCfg, err := config.LoadFile(logger, cliCfg.ConfigFile)
		if err != nil {
			level.Error(logger).Log("err", err, "msg", "Error loading config file")
			return nil, err
		}
		cfg = fileCfg
	}
	// Merge overwritting cliCfg into cfg
	if err := mergo.MergeWithOverwrite(cfg, cliCfg); err != nil {
		level.Error(logger).Log("err", err, "msg", "Error merging config file with flags")
		return nil, err
	}

	if cliCfg.Read.Delay == 0 {
		cfg.Read.Delay = cliCfg.Read.Delay
	}

	if cliCfg.Read.Timeout == 0 {
		cfg.Read.Timeout = cliCfg.Read.Timeout
	}

	if cliCfg.Write.Timeout == 0 {
		cfg.Write.Timeout = cliCfg.Write.Timeout
	}

	return cfg, nil
}

func main() {
	cliCfg := config.ParseCommandLine()

	logger := promlog.New(&promlog.Config{Level: &cliCfg.LogLevel})
	level.Info(logger).Log("msg", "Starting graphite-remote-adapter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	// Load the config once.
	cfg, err := reload(cliCfg, logger)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Error first loading config")
		return
	}

	webHandler := web.New(log.With(logger, "component", "web"), cfg)
	if err := webHandler.ApplyConfig(cfg); err != nil {
		level.Error(logger).Log("err", err, "msg", "Error applying webHandler config")
		return
	}

	// Tooling to dynamically reload the config for each clients.
	hup := make(chan os.Signal)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				cfg, err := reload(cliCfg, logger)
				if err != nil {
					level.Error(logger).Log("err", err, "msg", "Error reloading config")
					continue
				}
				if err := webHandler.ApplyConfig(cfg); err != nil {
					level.Error(logger).Log("err", err, "msg", "Error applying webHandler config")
					continue
				}
				level.Info(logger).Log("msg", "Reloaded config file")
			case rc := <-webHandler.Reload():
				cfg, err := reload(cliCfg, logger)
				if err != nil {
					level.Error(logger).Log("err", err, "msg", "Error reloading config")
					rc <- err
				} else if err := webHandler.ApplyConfig(cfg); err != nil {
					level.Error(logger).Log("err", err, "msg", "Error applying webHandler config")
					rc <- err
				} else {
					level.Info(logger).Log("msg", "Reloaded config file")
					rc <- nil
				}
			}
		}
	}()

	err = webHandler.Run()
	if err != nil {
		level.Warn(logger).Log("err", err)
	}
	level.Info(logger).Log("msg", "See you next time!")
}
