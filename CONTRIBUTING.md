# How to contribute to Graphite Remote Adapter


## Did you find a bug?

1. Please ensure the bug was not already reported by searching the 
   [Github Issues](https://github.com/criteo/graphite-remote-adapter/issues).
2. If you are unable to find an open issue addressing the problem, 
   [open a new one](https://github.com/criteo/graphite-remote-adapter/issues/new). 
   Make sure to include a *title and clear description* and as much relevant information as possible.
   
## Code contributions

Before working on a code contribution, we advise you to
[create an issue](https://github.com/criteo/graphite-remote-adapter/issues/new) describing your intent. That will let
us discuss together on how to do it best. 

### Building the project

You will need those dependencies:

* `make`
* a [Go distribution](https://golang.org/doc/install)

To build the project and produce the binaries, run the command

```bash
make
```

There are also some tools to help matching quality standards:

* `make format` to reformat the code (required to make the build success)
* `make style` to check code style and run a linter
* `make vet` to report suspicious constructs


### Dependency management

Dependencies are described in the `vendor/vendor.json` file. To manage them, use the
[govendor tool](https://github.com/kardianos/govendor).
