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
	"regexp"
	"text/template"
	"time"

	"encoding/json"

	"github.com/criteo/graphite-remote-adapter/utils"
	utils_tmpl "github.com/criteo/graphite-remote-adapter/utils/template"
	"github.com/prometheus/common/model"

	"gopkg.in/yaml.v2"
)

// DefaultConfig is the default graphite configuration.
var DefaultConfig = Config{
	DefaultPrefix:        "",
	EnableTags:           false,
	UseOpenMetricsFormat: false,
	Write: WriteConfig{
		CarbonAddress:           "",
		CarbonTransport:         "tcp",
		CarbonReconnectInterval: 1 * time.Hour,
		EnablePathsCache:        true,
		PathsCacheTTL:           1 * time.Hour,
		PathsCachePurgeInterval: 2 * time.Hour,
	},
	Read: ReadConfig{
		URL:           "",
		MaxPointDelta: time.Duration(0),
	},
}

// Config is the graphite configuration.
type Config struct {
	Write                WriteConfig `yaml:"write,omitempty" json:"write,omitempty"`
	Read                 ReadConfig  `yaml:"read,omitempty" json:"read,omitempty"`
	DefaultPrefix        string      `yaml:"default_prefix,omitempty" json:"default_prefix,omitempty"`
	EnableTags           bool        `yaml:"enable_tags,omitempty" json:"enable_tags,omitempty"`
	UseOpenMetricsFormat bool        `yaml:"openmetrics,omitempty" json:"openmetrics,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
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
	return utils.CheckOverflow(c.XXX, "graphite config")
}

// ReadConfig is the read graphite configuration.
type ReadConfig struct {
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
	// If set, MaxPointDelta is used to linearly interpolate intermediate points.
	// It helps support prom1.x reading metrics with larger retention than staleness delta.
	MaxPointDelta time.Duration `yaml:"max_point_delta,omitempty" json:"max_point_delta,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ReadConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ReadConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return utils.CheckOverflow(c.XXX, "readConfig")
}

// WriteConfig is the write graphite configuration.
type WriteConfig struct {
	CarbonAddress           string                 `yaml:"carbon_address,omitempty" json:"carbon_address,omitempty"`
	CarbonTransport         string                 `yaml:"carbon_transport,omitempty" json:"carbon_transport,omitempty"`
	CarbonReconnectInterval time.Duration          `yaml:"carbon_reconnect_interval,omitempty" json:"carbon_reconnect_interval,omitempty"`
	EnablePathsCache        bool                   `yaml:"enable_paths_cache,omitempty" json:"enable_paths_cache,omitempty"`
	PathsCacheTTL           time.Duration          `yaml:"paths_cache_ttl,omitempty" json:"paths_cache_ttl,omitempty"`
	PathsCachePurgeInterval time.Duration          `yaml:"paths_cache_purge_interval,omitempty" json:"paths_cache_purge_interval,omitempty"`
	TemplateData            map[string]interface{} `yaml:"template_data,omitempty" json:"template_data,omitempty"`
	Rules                   []*Rule                `yaml:"rules,omitempty" json:"rules,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WriteConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain WriteConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return utils.CheckOverflow(c.XXX, "writeConfig")
}

// LabelSet pairs a LabelName to a LabelValue.
type LabelSet map[model.LabelName]model.LabelValue

// LabelSetRE defines pairs like LabelSet but does regular expression
type LabelSetRE map[model.LabelName]Regexp

// Rule defines a templating rule that customize graphite path using the
// Tmpl if a metric matching the labels exists.
type Rule struct {
	Tmpl     Template   `yaml:"template,omitempty" json:"template,omitempty"`
	Match    LabelSet   `yaml:"match,omitempty" json:"match,omitempty"`
	MatchRE  LabelSetRE `yaml:"match_re,omitempty" json:"match_re,omitempty"`
	Continue bool       `yaml:"continue,omitempty" json:"continue,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Rule) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Rule
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	return utils.CheckOverflow(r.XXX, "rule")
}

// Template is a parsable template.
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
	template, err := template.New("").Funcs(utils_tmpl.TmplFuncMap).Parse(s)
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
