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
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"text/template"

	"encoding/json"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"gopkg.in/yaml.v2"
)

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
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

// DefaultGlobalConfig provides global default values.
var DefaultConfig = Config{}

type Config struct {
	Template_data map[string]interface{} `yaml:"template_data,omitempty" json:"template_data,omitempty"`
	Rules         []*Rule                `yaml:"rules,omitempty" json:"rules,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`

	// original is the input from which the Config was parsed.
	original string
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultConfig
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return checkOverflow(c.XXX, "config")
}

type LabelSet map[model.LabelName]model.LabelValue
type LabelSetRE map[model.LabelName]Regexp

type Rule struct {
	Tmpl     Template                             `yaml:"template,omitempty" json:"template,omitempty"`
	Match    map[model.LabelName]model.LabelValue `yaml:"match,omitempty" json:"match,omitempty"`
	MatchRE  map[model.LabelName]Regexp           `yaml:"match_re,omitempty" json:"match_re,omitempty"`
	Continue bool                                 `yaml:"continue,omitempty" json:"continue,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Rule) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Rule
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	return checkOverflow(r.XXX, "rule")
}

type Template struct {
	*template.Template
	original string
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
	tmpl.original = s
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (tmpl Template) MarshalYAML() (interface{}, error) {
	return tmpl.original, nil
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
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

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	return re.String(), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (re Regexp) MarshalJSON() ([]byte, error) {
	if re.Regexp != nil {
		return json.Marshal(re.String())
	}
	return nil, nil
}
