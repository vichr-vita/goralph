package main

import (
	"fmt"
	"os"

	"github.com/vichr-vita/goralph/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand()
	if err := cmd.Execute(); err != nil {
		if cli.CommandWantsJSON(cmd) {
			_ = cli.WriteErrorJSON(os.Stdout, err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(cli.ExitCodeForError(err))
	}
}
