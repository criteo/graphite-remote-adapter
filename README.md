# Graphite Remote storage adapter [![Build Status](https://travis-ci.org/criteo/graphite-remote-adapter.svg?branch=master)](https://travis-ci.org/criteo/graphite-remote-adapter)

This is a read/write adapter that receives samples via Prometheus's remote write
protocol and stores them in Graphite.

It is based on [remote_storage_adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter)

### Compiling the binary

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/criteo/graphite-remote-adapter/cmd/...
# cd $GOPATH/src/github.com/criteo/graphite-remote-adapter
$ graphite-remote-adapter -config.file=<your_file>
```

Or checkout the source code and build manually:

```
$ mkdir -p $GOPATH/src/github.com/criteo
$ cd $GOPATH/src/github.com/criteo
$ git clone https://github.com/criteo/graphite-remote-adapter.git
$ cd graphite-remote-adapter
$ make build
$ ./graphite-remote-adapter -graphite--address=localhost:2003
```

## Running

Graphite example:

```
./graphite-remote-adapter \
  -carbon-address localhost:2001 \
  -graphite-url http://localhost:8080/ \
  -read-timeout 10s -write-timeout 5s \
  -graphite-prefix prometheus.
```

To show all flags:

```
./graphite-storage-adapter -h
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
