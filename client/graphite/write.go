// Copyright 2015 The Prometheus Authors
// Copyright 2017 Thibault Chataigner <thibault.chataigner@gmail.com>
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

package graphite

import (
	"bytes"
	"fmt"
	"math"
	"net"

	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
)

func (c *Client) prepareDataPoint(path string, s *model.Sample) string {
	t := float64(s.Timestamp.UnixNano()) / 1e9
	v := float64(s.Value)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		level.Debug(c.logger).Log(
			"value", v, "sample", s, "msg", "cannot send a value, skipping sample")
		c.ignoredSamples.Inc()
		return ""
	}
	return fmt.Sprintf("%s %f %f\n", path, v, t)
}

// Write sends a batch of samples to Graphite.
func (c *Client) Write(samples model.Samples) error {
	level.Debug(c.logger).Log(
		"num_samples", len(samples), "storage", c.Name(), "msg", "Remote write")
	if c.cfg.Write.CarbonAddress == "" {
		return nil
	}

	conn, err := net.DialTimeout(c.cfg.Write.CarbonTransport, c.cfg.Write.CarbonAddress, c.writeTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	var buf bytes.Buffer
	for _, s := range samples {
		paths := pathsFromMetric(s.Metric, c.cfg.DefaultPrefix, c.cfg.Write.Rules, c.cfg.Write.TemplateData)
		for _, k := range paths {
			if str := c.prepareDataPoint(k, s); str != "" {
				fmt.Fprint(&buf, str)
				level.Debug(c.logger).Log("line", str, "msg", "Sending")
			}
		}
	}

	_, err = conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}
