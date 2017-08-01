package graphite

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPrepareUrl(t *testing.T) {
	url, err := prepareUrl("host:1234", "a", map[string]string{})
	assert.Nil(t, err, "No error")
	assert.Equal(t, "http", url.Scheme, "http scheme")
	assert.Equal(t, "host:1234", url.Host, "host:port")
	assert.Empty(t, url.User, "empty userinfo")

	url, err = prepareUrl("http://host:1234", "a", map[string]string{})
	assert.Nil(t, err, "No error")
	assert.Equal(t, "http", url.Scheme, "http scheme")
	assert.Equal(t, "host:1234", url.Host, "host:port")
	assert.Empty(t, url.User, "empty userinfo")

	url, err = prepareUrl("http://guest:guest@host:1234", "a", map[string]string{})
	assert.Nil(t, err, "No error")
	assert.Equal(t, "http", url.Scheme, "http scheme")
	assert.Equal(t, "host:1234", url.Host, "host:port")
	assert.NotEmpty(t, url.User, "userinfo are used")

	url, err = prepareUrl("https://guest:guest@host:1234", "a", map[string]string{})
	assert.Nil(t, err, "No Error")
	assert.Equal(t, "https", url.Scheme, "https scheme")
	assert.Equal(t, "host:1234", url.Host, "host:port")
	assert.NotEmpty(t, url.User, "userinfo are used")
}
