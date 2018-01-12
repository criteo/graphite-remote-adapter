# Graphite Remote storage adapter [![Build Status](https://travis-ci.org/criteo/graphite-remote-adapter.svg?branch=master)](https://travis-ci.org/criteo/graphite-remote-adapter)

This is a read/write adapter that receives samples via Prometheus's remote write
protocol and stores them in remote storage like Graphite.

It is based on [remote_storage_adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter)

### Compiling the binary

You can either `go get` it:

```
$ go get github.com/criteo/graphite-remote-adapter/...
$ cd $GOPATH/src/github.com/criteo/graphite-remote-adapter
$ make build
$ ./graphite-remote-adapter --graphite.read.url='http://localhost:8080' --graphite.write.carbon-address=localhost:2003
```

Or checkout the source code and build manually:

```
$ mkdir -p $GOPATH/src/github.com/criteo
$ cd $GOPATH/src/github.com/criteo
$ git clone https://github.com/criteo/graphite-remote-adapter.git
$ cd graphite-remote-adapter
$ make build
$ ./graphite-remote-adapter --graphite.read.url='http://localhost:8080' --graphite.write.carbon-address=localhost:2003
```

## Running

Graphite example:

```
./graphite-remote-adapter \
  --graphite.write.carbon-address=localhost:2001 \
  --graphite.read.url='http://guest:guest@localhost:8080' \
  --read.timeout= 10s --write.timeout= 5s \
  --read.delay 3600s \
  --graphite.default-prefix prometheus.
```

To show all flags:

```
./graphite-remote-adapter -h
```

## Example
You can provide some configuration parameters either as flags or in a configuration file. If defined in both, the flag is used.
In addtion, you can fill the configuration file with Graphite specific parameters. You can indeed defined customized paths/behaviors for remote-write into Graphite.

This is an example configuration that should cover most relevant aspects of the YAML configuration format.

```yaml
web:
  listen_address: "0.0.0.0:9201"
  telemetry_path: "/metrics"
write:
  timeout: 5m
read:
  timeout: 5m
  delay: 1h
  ignore_error: true
graphite:
  default_prefix: test.prefix.
  enable_tags: false
  read:
    url: http://localhost:8888
  write:
    carbon_address: localhost:2003
    carbon_transport: tcp
    carbon_reconnect_interval: 5m
    enable_paths_cache: true
    paths_cache_ttl: 1h
    paths_cache_purge_interval: 2h
    template_data:
      var1:
        foo: bar
      var2: foobar

    rules:
    - match:
        owner: team-X
      match_re:
        service: ^(foo1|foo2|baz)$
      template: '{{.var1.foo}}.graphite.path.host.{{.labels.owner}}.{{.labels.service}}{{if ne .labels.env "prod"}}.{{.labels.env}}{{end}}'
      continue: true
    - match:
        owner: team-X
        env:   prod
      template: 'bla.bla.{{.labels.owner | escape}}.great.{{.var2}}'
      continue: true
    - match:
        owner: team-Z
      continue: false

```

## Support for Tags

Graphite 1.1.0 supports tags: http://graphite.readthedocs.io/en/latest/tags.html, you can
enable support for tags in the remote adapter with `--graphite.enable-tags` or in the
configuration file.

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
