package deploy_test

import (
	"flag"
	"github.com/mever/gocharm/v2/deploy"
	"github.com/mever/gocharm/v2/hook"
)

func ExampleRunMain() {
	// This example demonstrates a complete charm
	// that does nothing at all. This code would usually be
	// placed in the main function.

	r := hook.NewRegistry()
	r.SetCharmInfo(hook.CharmInfo{
		Name:        "example",
		Summary:     "An example charm",
		Description: "This charm does nothing",
	})

	// Register any hooks and other charm logic here.

	deploy.MainFlags()
	flag.Parse()
	deploy.RunMain(r)

	// Could do other non-charm-related stuff here. For example,
	// a command could both act as a charm and as a locally runnable
	// server.
}
