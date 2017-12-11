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

package config

import (
	"io/ioutil"
	"regexp"
	"testing"
	"text/template"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/criteo/graphite-remote-adapter/utils"
)

var (
	expectedConf = &Config{
		DefaultPrefix: "test.prefix.",
		Read: ReadConfig{
			URL: "greatGraphiteWebURL",
		},
		Write: WriteConfig{
			CarbonAddress:           "greatCarbonAddress",
			CarbonTransport:         "tcp",
			EnablePathsCache:        true,
			PathsCacheTTL:           18 * time.Minute,
			PathsCachePurgeInterval: 42 * time.Minute,
			TemplateData: map[string]interface{}{
				"site_mapping": map[string]string{"eu-par": "fr_eqx"},
			},
			Rules: []*Rule{
				{
					Match: LabelSet{
						"owner": "team-X",
					},
					MatchRE: LabelSetRE{
						"service": prepareExpectedRegexp("^(foo1|foo2|baz)$"),
					},
					Continue: true,
					Tmpl:     prepareExpectedTemplate("great.graphite.path.host.{{.labels.owner}}.{{.labels.service}}{{if ne .labels.env \"prod\"}}.{{.labels.env}}{{end}}"),
				},
				{
					Match: LabelSet{
						"owner": "team-X",
						"env":   "prod",
					},
					Continue: true,
					Tmpl:     prepareExpectedTemplate("bla.bla.{{.labels.owner | escape}}.great.path"),
				},
				{
					Match: LabelSet{
						"owner": "team-Z",
					},
					Continue: false,
				},
			},
		},
	}
	testConfigFile = "testdata/graphite.good.yml"
)

func prepareExpectedRegexp(s string) Regexp {
	r, _ := regexp.Compile("^(?:" + s + ")$")
	return Regexp{r}
}

func prepareExpectedTemplate(s string) Template {
	t, _ := template.New("").Funcs(utils.TmplFuncMap).Parse(s)
	return Template{t, s}
}

func TestUnmarshalConfig(t *testing.T) {
	cfg := &Config{}
	content, _ := ioutil.ReadFile(testConfigFile)
	err := yaml.Unmarshal([]byte(string(content)), cfg)
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	if cfg.String() != expectedConf.String() {
		t.Fatalf("%s: unexpected config result: \n%s\nExpecting:\n%s",
			"testdata/conf.good.yml", cfg.String(), expectedConf.String())
	}
}
