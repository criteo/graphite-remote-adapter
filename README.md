# Graphite Remote storage adapter [![Build Status](https://travis-ci.org/criteo/graphite-remote-adapter.svg?branch=master)][travis]

This is a read/write adapter that receives samples via Prometheus's remote write
protocol and stores them in Graphite.

It is based on [remote_storage_adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter)

### Compiling the binary

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/criteo/graphite-remote-adapter/cmd/...
# cd $GOPATH/src/github.com/criteo/graphite-remote-adapter
$ alertmanager -config.file=<your_file>
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
./graphite_storage_adapter -graphite-address=localhost:8080
```

To show all flags:

```
./graphite_storage_adapter -h
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
