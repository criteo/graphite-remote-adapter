# Graphite Remote storage adapter [![Build Status](https://travis-ci.org/criteo/graphite-remote-adapter.svg?branch=master)](https://travis-ci.org/criteo/graphite-remote-adapter)

This is a read/write adapter that receives samples via Prometheus's remote write
protocol and stores them in Graphite.

It is based on [remote_storage_adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter)

### Compiling the binary

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/criteo/graphite-remote-adapter/cmd/...
$ cd $GOPATH/src/github.com/criteo/graphite-remote-adapter
$ make build
$ ./graphite-remote-adapter -graphite-url=localhost:2003
```

Or checkout the source code and build manually:

```
$ mkdir -p $GOPATH/src/github.com/criteo
$ cd $GOPATH/src/github.com/criteo
$ git clone https://github.com/criteo/graphite-remote-adapter.git
$ cd graphite-remote-adapter
$ make build
$ ./graphite-remote-adapter -graphite-url=localhost:2003
```

## Running

Graphite example:

```
./graphite-remote-adapter \
  -carbon-address localhost:2001 \
  -graphite-url localhost:8080 \
  -read-timeout 10s -write-timeout 5s \
  -read-delay 3600s \
  -graphite-prefix prometheus.
```

To show all flags:

```
./graphite-remote-adapter -h
```

## Example
This is an example configuration that should cover most relevant aspects of the YAML configuration format.

```yaml
template_data:
    site_mapping:
        eu-par: fr_eqx

rules:
    - match:
        owner: team-X
      match_re:
        service: ^(foo1|foo2|baz)$
      template: 'great.graphite.path.host.{{.labels.owner}}.{{.labels.service}}{{if ne .labels.env "prod"}}.{{.labels.env}}{{end}}'
      continue: true
    - match:
        owner: team-X
        env:   prod
      template: 'bla.bla.{{.labels.owner}}.great.path'
      continue: true
    - match:
        env: team-Z
      continue: false
```

## Configuring Prometheus

To configure Prometheus to send samples to this binary, add the following to your `prometheus.yml`:

```yaml
# Remote write configuration.
remote_write:
  - url: "http://localhost:9201/write"

# Remote read configuration.
remote_read:
  - url: "http://localhost:9201/read"
```
