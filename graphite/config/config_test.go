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
	"regexp"
	"testing"
	"text/template"
)

func prepareExpectedRegexp(s string) Regexp {
	r, _ := regexp.Compile("^(?:" + s + ")$")
	return Regexp{r}
}

func prepareExpectedTemplate(s string) Template {
	t, _ := template.New("").Funcs(TmplFuncMap).Parse(s)
	return Template{t, s}
}

var expectedConf = &Config{
	Template_data: map[string]interface{}{
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
	original: "",
}

func TestLoadConfigFile(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}
	c.original = ""

	if c.String() != expectedConf.String() {
		t.Fatalf("%s: unexpected config result: \n%s\nExpecting:\n%s",
			"testdata/conf.good.yml", c.String(), expectedConf.String())
	}
}
