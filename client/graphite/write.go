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
	"net"
	"net/http"
	"time"

	gpaths "github.com/criteo/graphite-remote-adapter/client/graphite/paths"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
)

const udpMaxBytes = 1024

func (c *Client) connectToCarbon() (net.Conn, error) {
	if c.carbonCon != nil {
		if time.Since(c.carbonLastReconnectTime) < c.cfg.Write.CarbonReconnectInterval {
			// Last reconnect is not too long ago, re-use the connection.
			return c.carbonCon, nil
		}
		level.Debug(c.logger).Log(
			"last", c.carbonLastReconnectTime,
			"msg", "Reinitializing the connection to carbon")
		c.disconnectFromCarbon()
	}

	level.Debug(c.logger).Log(
		"transport", c.cfg.Write.CarbonTransport,
		"address", c.cfg.Write.CarbonAddress,
		"timeout", c.writeTimeout,
		"msg", "Connecting to carbon")
	conn, err := net.DialTimeout(c.cfg.Write.CarbonTransport, c.cfg.Write.CarbonAddress, c.writeTimeout)
	if err != nil {
		c.carbonCon = nil
	} else {
		c.carbonLastReconnectTime = time.Now()
		c.carbonCon = conn
	}

	return c.carbonCon, err
}

func (c *Client) disconnectFromCarbon() {
	if c.carbonCon != nil {
		c.carbonCon.Close()
	}
	c.carbonCon = nil
}

func (c *Client) prepareWrite(samples model.Samples, r *http.Request) ([]*bytes.Buffer, error) {
	level.Debug(c.logger).Log(
		"num_samples", len(samples), "storage", c.Name(), "msg", "Remote write")

	graphitePrefix, err := c.getGraphitePrefix(r)
	if err != nil {
		level.Warn(c.logger).Log("prefix", graphitePrefix, "err", err)
		return nil, err
	}

	currentBuf := bytes.NewBufferString("")
	bytesBuffers := []*bytes.Buffer{currentBuf}
	for _, s := range samples {
		datapoints, err := gpaths.ToDatapoints(s, c.format, graphitePrefix, c.cfg.Write.Rules, c.cfg.Write.TemplateData)
		if err != nil {
			level.Debug(c.logger).Log("sample", s, "err", err)
			c.ignoredSamples.Inc()
			continue
		}
		for _, str := range datapoints {
			if c.cfg.Write.CarbonTransport == "udp" && (currentBuf.Len()+len(str)) > udpMaxBytes {
				currentBuf = bytes.NewBufferString("")
				bytesBuffers = append(bytesBuffers, currentBuf)
			}
			fmt.Fprint(currentBuf, str)
			level.Debug(c.logger).Log("line", str, "msg", "Sending")
		}
	}
	return bytesBuffers, nil
}

// Write implements the client.Writer interface.
func (c *Client) Write(samples model.Samples, r *http.Request, dryRun bool) ([]byte, error) {
	if c.cfg.Write.CarbonAddress == "" {
		return []byte("Skipped: Not set carbon address."), nil
	}

	bytesBuffers, err := c.prepareWrite(samples, r)
	if err != nil {
		return nil, err
	}

	if dryRun {
		dryRunResponse := make([]byte, 0)
		for _, buf := range bytesBuffers {
			dryRunResponse = append(dryRunResponse, buf.Bytes()...)
		}
		return dryRunResponse, nil

	}
	// We are going to use the socket, lock it.
	c.carbonConLock.Lock()
	defer c.carbonConLock.Unlock()

	for _, buf := range bytesBuffers {
		conn, err := c.connectToCarbon()
		if err != nil {
			return nil, err
		}

		_, err = conn.Write(buf.Bytes())
		if err != nil {
			c.disconnectFromCarbon()
			return nil, err
		}
	}
	return []byte("Done."), nil
}
