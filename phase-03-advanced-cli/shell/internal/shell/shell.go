package shell

// shell.go — the Shell type: shared state plus the top-level entry points that
// tie the three stages (tokenize -> parse -> execute) together.

import (
	"fmt"
	"io"
	"os"
)

// Shell holds everything that must survive across commands within one session.
//
// 🐍→🐹 The In/Out/Err fields are io.Reader/io.Writer interfaces, not concrete
// files. That's the same trick we used across the Phase 2 tools: production wires
// them to os.Stdin/os.Stdout/os.Stderr, but tests wire them to bytes.Buffer so
// they can assert on captured output without spawning a terminal.
type Shell struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	vars       map[string]string // shell-local variables (NOT exported to children)
	lastStatus int               // exit code of the most recent command -> $?
	cwd        string            // our own working directory (see cd builtin)
	shouldExit bool              // set by the `exit` builtin to stop the REPL
}

// New builds a Shell wired to the given streams.
func New(in io.Reader, out, errw io.Writer) *Shell {
	cwd, _ := os.Getwd()
	return &Shell{
		In:   in,
		Out:  out,
		Err:  errw,
		vars: map[string]string{},
		cwd:  cwd,
	}
}

// LastStatus exposes $? to callers (e.g. main, for the process exit code).
func (sh *Shell) LastStatus() int { return sh.lastStatus }

// ShouldExit reports whether the `exit` builtin asked us to stop.
func (sh *Shell) ShouldExit() bool { return sh.shouldExit }

// RunLine is the full pipeline for one line of input: tokenize -> parse ->
// execute. Syntax errors are reported on stderr and yield status 2 (the
// conventional "usage/syntax error" code used throughout this repo).
func (sh *Shell) RunLine(line string) int {
	toks, err := tokenize(line)
	if err != nil {
		fmt.Fprintf(sh.Err, "gosh: %v\n", err)
		sh.lastStatus = 2
		return 2
	}
	if len(toks) == 0 {
		return sh.lastStatus // blank line / comment-only: status unchanged
	}
	list, err := parse(toks)
	if err != nil {
		fmt.Fprintf(sh.Err, "gosh: %v\n", err)
		sh.lastStatus = 2
		return 2
	}
	return sh.execList(list)
}
