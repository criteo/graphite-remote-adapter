package config

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/prometheus/common/log"
	yaml "gopkg.in/yaml.v2"

	graphite "github.com/criteo/graphite-remote-adapter/graphite/config"
	"github.com/criteo/graphite-remote-adapter/utils"
)

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
var DefaultConfig = Config{
	Web: webOptions{
		ListenAddress: "0.0.0.0:9201",
		TelemetryPath: "/metrics",
	},
	Read: readOptions{
		Timeout:     5 * time.Minute,
		Delay:       1 * time.Hour,
		IgnoreError: true,
	},
	Write: writeOptions{
		Timeout: 5 * time.Minute,
	},
}

type Config struct {
	ConfigFile string
	Web        webOptions      `yaml:"web,omitempty" json:"web,omitempty"`
	Read       readOptions     `yaml:"read,omitempty" json:"read,omitempty"`
	Write      writeOptions    `yaml:"write,omitempty" json:"write,omitempty"`
	Graphite   graphite.Config `yaml:"graphite,omitempty" json:"graphite,omitempty"`

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
	return utils.CheckOverflow(c.XXX, "config")
}

type webOptions struct {
	ListenAddress string `yaml:"listen_address,omitempty" json:"listen_address,omitempty"`
	TelemetryPath string `yaml:"telemetry_path,omitempty" json:"telemetry_path,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (opts *webOptions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain webOptions
	if err := unmarshal((*plain)(opts)); err != nil {
		return err
	}

	return utils.CheckOverflow(opts.XXX, "webOptions")
}

type readOptions struct {
	Timeout     time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Delay       time.Duration `yaml:"delay,omitempty" json:"delay,omitempty"`
	IgnoreError bool          `yaml:"ignore_error,omitempty" json:"ignore_error,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (opts *readOptions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain readOptions
	if err := unmarshal((*plain)(opts)); err != nil {
		return err
	}

	return utils.CheckOverflow(opts.XXX, "readOptions")
}

type writeOptions struct {
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (opts *writeOptions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain writeOptions
	if err := unmarshal((*plain)(opts)); err != nil {
		return err
	}

	return utils.CheckOverflow(opts.XXX, "writeOptions")
}
