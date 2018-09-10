package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_parseUnittestConfig(t *testing.T) {
	const testInputConfig = `
config_file: config_file.yml

tests:
  - name: "Test label"
    input: |
        # Use the Prometheus exposition text format
        toto{foo="bar", cluster="test"} 42
        toto{foo="bar", cluster="canary"} 34
        # You can even force a given timestamp
        toto{foo="bazz", cluster="canary"} 18 1528819131000
    output: |
        toto.my.templated.path.test.foo.bar.lulu 42.000000 1570802650
        toto.canary.other.template.bar 34.000000 1570802650
        toto.canary.other.template.bazz 18.000000 1528819131

  - name: "Other test"
    input: |
        foo{bar="baz"} 10
    output: |
        foo.bar.baz.lol 10 1528819131000`

	expectedConfig := &unittestConfig{
		ConfigFile: "config_file.yml",
		Tests: []*testConfig{
			{
				Name: "Test label",
				Input: `# Use the Prometheus exposition text format
toto{foo="bar", cluster="test"} 42
toto{foo="bar", cluster="canary"} 34
# You can even force a given timestamp
toto{foo="bazz", cluster="canary"} 18 1528819131000
`,
				Output: `toto.my.templated.path.test.foo.bar.lulu 42.000000 1570802650
toto.canary.other.template.bar 34.000000 1570802650
toto.canary.other.template.bazz 18.000000 1528819131
`,
			},
			{
				Name:   "Other test",
				Input:  "foo{bar=\"baz\"} 10\n",
				Output: "foo.bar.baz.lol 10 1528819131000",
			},
		},
	}

	const testWrongInputFile = `non valid YAML format`

	t.Run("parseUnittestConfig should parse a properly formatted file", func(t *testing.T) {
		config, err := parseUnittestConfig([]byte(testInputConfig))

		assert.Nil(t, err)
		assert.Equal(t, expectedConfig, config)

	})

	t.Run("parseUnittestConfig should do something with a wrong input file", func(t *testing.T) {
		config, err := parseUnittestConfig([]byte(testWrongInputFile))

		assert.Nil(t, config)
		assert.Contains(t, fmt.Sprintf("%s", err), "cannot unmarshal")
	})
}
