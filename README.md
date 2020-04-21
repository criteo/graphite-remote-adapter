# Graphite Remote storage adapter [![Build Status](https://travis-ci.org/criteo/graphite-remote-adapter.svg?branch=master)](https://travis-ci.org/criteo/graphite-remote-adapter)

This is a read/write adapter that receives samples via Prometheus's remote write
protocol and stores them in remote storage like Graphite.

It is based on [remote_storage_adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter)

### Compiling the binary

You can either `go get` it:

```
$ go get -d github.com/criteo/graphite-remote-adapter/...
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
  --read.timeout=10s --write.timeout=5s \
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

Since the 0.0.15, a custom prefix can be set in the query string and this will replace the default one. This could be useful if you are using the graphite remote adapter for multiple Prometheus instances with different prefix.

```yaml
# Remote write configuration.
remote_write:
  - url: "http://localhost:9201/write?graphite.default-prefix=customprefix."

# Remote read configuration.
remote_read:
  - url: "http://localhost:9201/read?graphite.default-prefix=customprefix."
```

## Testing

You can test the graphite-remote-adapter behavior or its configuration using the second binary named **ratool** for remote-adapter tool.
Here are two examples:

#### Integration test (manual end-to-end)

The remote-adapter tool will read an input file in Prometheus exposition text format;
translate it in WriteRequest using compressed protobuf format; and send it to
the graphite-remote-adapter url on its /write endpoint.
No need to run a Prometheus instance to test it anymore:

file -> ratool -> graphite-remote-adapter -> nc
```
$ make build
$ cat cmd/ratool/input.metrics.example
  # Use the Prometheus exposition text format
  toto{foo="bar", cluster="test"} 42
  toto{foo="bar", cluster="canary"} 34
  # You can even force a given timestamp
  toto{foo="bazz", cluster="canary"} 18 1528819131000
$ ./graphite-remote-adapter --graphite.write.carbon-address ':8888' --log.level debug &
$ nc -l 0.0.0.0 8888 -w 1 > out.txt
$ ./ratool mock-write --metrics.file cmd/ratool/input.metrics.example --remote-adapter.url 'http://localhost:9201'
$ cat out.txt
  toto.cluster.test.foo.bar 42.000000 1570803131
  toto.cluster.canary.foo.bar 34.000000 1570803131
  toto.cluster.canary.foo.bazz 18.000000 1528819131
```

#### Unittests (automated config unittests)

If you want to unit test your configurations without requiring any network, define a file for each configuration you
want to test.

Example:

```yaml
config_file: config.yml
tests:
  - name: "Test label"
    input: |
        # Use the Prometheus exposition text format
        toto{foo="bar", cluster="test"} 42 1570802650000
        toto{foo="bar", cluster="canary"} 34 1570802650000
        toto{foo="bazz", cluster="canary"} 18 1528819131000
    output: |
        toto.my.templated.path.test.foo.bar.lulu 42.000000 1570802650
        toto.canary.other.template.bar 34.000000 1570802650
        toto.canary.other.template.bazz 18.000000 1528819131

  - name: "Other test"
    input: |
        foo{bar="baz"} 10
    output: |
        foo.bar.baz.lol 10 1528819131000
```

The path to `config_file` is relative to the test file.

> *Note:* timestamps do not have the same unit for input and output. Input uses a regular unix timestamp in 
> milliseconds, output is in seconds.

To run it:

```
$ make build
$ ./ratool unittest --test.file test_file.yml
```

The tool will exit with a non-zero code if the output of the remote adapter for the given configuration and the given 
input does not match the expected output (order of the lines is not checked). 

It also prints the diff on the standard error stream. 

Example of output:

```plain
./ratool unittest --config.file foo.yml --test.file bar.yml
# Testing foo.yml
## Test label
-toto.my.templated.path.test.foo.bar.lulu 42.000000 1570802650
-toto.canary.other.template.bar 34.000000 1570802650
-toto.canary.other.template.bazz 18.000000 1528819131
+toto.cluster.test.foo.bar 42.000000 1536658898
+
+toto.cluster.canary.foo.bar 34.000000 1536658898
+
+toto.cluster.canary.foo.bazz 18.000000 1528819131
## Other test
-foo.bar.baz.lol 10 1528819131000
+foo.bar.baz 10.000000 1536658898
```
