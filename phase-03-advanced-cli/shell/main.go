// Command gosh is a small from-scratch Unix shell — the Phase 3 capstone.
//
// Usage:
//
//	gosh                 # interactive REPL (prompt loop)
//	gosh -c "echo hi"    # run a single command string and exit
//	gosh script.sh       # run a script file line by line
//
// 🐍→🐹 Like every Go program, execution starts at func main in package main.
// We keep main tiny: parse the invocation mode, build a Shell wired to the real
// stdio streams, and translate the final $? into the process exit code. All the
// real logic lives in the internal/shell package so it can be unit-tested
// without a terminal.
package main

import (
	"fmt"
	"os"

	"gosh/internal/shell"
)

func main() {
	sh := shell.New(os.Stdin, os.Stdout, os.Stderr)

	args := os.Args[1:]
	switch {
	case len(args) >= 2 && args[0] == "-c":
		// `-c "command string"`: run it and exit with its status.
		sh.RunLine(args[1])

	case len(args) >= 1:
		// A script file argument.
		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "gosh: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		sh.RunReader(f)

	default:
		// No args: interactive shell.
		sh.RunInteractive()
	}

	os.Exit(sh.LastStatus())
}
