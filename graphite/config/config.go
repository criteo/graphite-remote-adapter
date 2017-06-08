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
	"text/template"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"gopkg.in/yaml.v2"
)

type FileConfig struct {
	Template_data map[string]interface{} `yaml:"template_data,omitempty" json:"template_data,omitempty"`
	Rules         []*Rule                `yaml:"rules,omitempty" json:"rules,omitempty"`

	// original is the input from which the FileConfig was parsed.
	original string
}

type Rule struct {
	Tmpl     Template                             `yaml:"template,omitempty" json:"template,omitempty"`
	Match    map[model.LabelName]model.LabelValue `yaml:"match,omitempty" json:"match,omitempty"`
	MatchRE  map[model.LabelName]Regexp           `yaml:"match_re,omitempty" json:"match_re,omitempty"`
	Continue bool                                 `yaml:"continue,omitempty" json:"continue,omitempty"`
}

type Template struct {
	*template.Template
}

type Regexp struct {
	*regexp.Regexp
}

// Load parses the YAML input s into a FileConfig.
func Load(s string) (*FileConfig, error) {
	cfg := &FileConfig{}
	err := yaml.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a FileConfig.
func LoadFile(filename string) (*FileConfig, error) {
	log.With("file", filename).Infof("Loading configuration file")
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tmpl *Template) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	template, err := template.New("").Funcs(TmplFuncMap).Parse(s)
	if err != nil {
		return err
	}
	tmpl.Template = template
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	return nil
}
