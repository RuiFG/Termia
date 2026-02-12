package main

import (
	"os"

	"github.com/termia/termia/cmd"
)

func main() {
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
