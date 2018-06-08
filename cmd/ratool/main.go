package main

import (
	"net/url"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const (
	helpRoot = `Tool to interact with a remote-adapter and its configuration.`
)

var (
	remoteAdapterURL *url.URL
)

func requireRemoteAdapterURL(pc *kingpin.ParseContext) error {
	// Return without error if any help flag is set.
	for _, elem := range pc.Elements {
		f, ok := elem.Clause.(*kingpin.FlagClause)
		if !ok {
			continue
		}
		name := f.Model().Name
		if name == "help" || name == "help-long" || name == "help-man" {
			return nil
		}
	}
	if remoteAdapterURL == nil {
		kingpin.Fatalf("required flag --remote-adapter.url not provided")
	}
	return nil
}

func main() {
	var (
		app = kingpin.New("ratool", helpRoot).DefaultEnvars()
	)
	app.Flag("remote-adapter.url", "Set a default remote-adapter url for each request.").URLVar(&remoteAdapterURL)
	app.GetFlag("help").Short('h')
	configureMockWriteCmd(app)

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}
}
