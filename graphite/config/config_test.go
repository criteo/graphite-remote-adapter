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
	"github.com/prometheus/common/model"
	"reflect"
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
	return Template{t}
}

var expectedConf = &FileConfig{
	Template_data: map[string]interface{}{
		"site_mapping": map[string]string{"eu-par": "fr_eqx"},
	},
	Rules: []*Rule{
		{
			Match: map[model.LabelName]model.LabelValue{
				"owner": "team-X",
			},
			MatchRE: map[model.LabelName]Regexp{
				"service": prepareExpectedRegexp("^(foo1|foo2|baz)$"),
			},
			Continue: true,
			Tmpl:     prepareExpectedTemplate("great.graphite.path.host.{{.labels.owner}}.{{.labels.service}}{{if ne .labels.env \"prod\"}}.{{.labels.env}}{{end}}"),
		},
		{
			Match: map[model.LabelName]model.LabelValue{
				"owner": "team-X",
				"env":   "prod",
			},
			Continue: true,
			Tmpl:     prepareExpectedTemplate("bla.bla.{{.labels.owner}}.great.path"),
		},
		{
			Match: map[model.LabelName]model.LabelValue{
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
	expectedConf.original = c.original

	if !reflect.DeepEqual(c.original, expectedConf.original) {
		t.Fatalf("%s: unexpected config result", "testdata/conf.good.yml")
	}
}
