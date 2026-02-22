package main

import (
	"os"

	"github.com/termia/termia/cmd"
	"github.com/termia/termia/internal/diagnostics"
)

func main() {
	diagnostics.Track("startup.main", nil)()
	if cmd.ShouldRunWrapper(os.Args[1:]) {
		if err := cmd.ExecuteWrapper(os.Args[1:]); err != nil {
			os.Exit(cmd.ExitCode(err))
		}
		return
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(cmd.ExitCode(err))
	}
}
