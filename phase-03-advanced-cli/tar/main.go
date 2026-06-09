// main.go — the command-line front end. It parses tar's classic flags
// (`-c`/`-x`/`-t` mode, plus `-f archive` and `-v`) and dispatches to the
// writer or reader. Following our Go-challenge convention, `main` is a thin
// wrapper around `run(args, stdout, stderr) int` so the whole CLI is testable
// without spawning a subprocess.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options holds the parsed command line.
type options struct {
	create  bool // -c
	extract bool // -x
	list    bool // -t
	verbose bool // -v
	file    string // -f <archive>  ("" or "-" means stdin/stdout)
	paths   []string
}

// Exit codes follow the repo convention: 0 success, 1 domain/IO failure,
// 2 usage error.
const (
	exitOK    = 0
	exitErr   = 1
	exitUsage = 2
)

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}

	switch {
	case opts.create:
		return doCreate(opts, stdout, stderr)
	case opts.list:
		return doList(opts, stdout, stderr)
	case opts.extract:
		return doExtract(opts, stderr)
	default:
		fmt.Fprintln(stderr, "tar: must specify one of -c, -x, or -t")
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
}

// parseArgs is a small hand-rolled parser (same approach as the other Go tools
// in this repo) so we can accept bundled short flags like `-cvf archive.tar`,
// the exact ergonomics people expect from real tar.
func parseArgs(args []string) (options, error) {
	var opts options
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break // first non-flag: the rest are paths
		}

		// Walk each character in a bundle (e.g. "-cvf").
		for j := 1; j < len(a); j++ {
			switch a[j] {
			case 'c':
				opts.create = true
			case 'x':
				opts.extract = true
			case 't':
				opts.list = true
			case 'v':
				opts.verbose = true
			case 'f':
				// -f takes the NEXT argument as the archive name. It must be
				// the last flag in its bundle (real tar requires this too).
				if j != len(a)-1 {
					return opts, fmt.Errorf("-f must be the last flag in a group")
				}
				if i+1 >= len(args) {
					return opts, fmt.Errorf("-f requires an archive filename")
				}
				i++
				opts.file = args[i]
			default:
				return opts, fmt.Errorf("unknown flag: -%c", a[j])
			}
		}
	}

	opts.paths = args[i:]

	// Exactly one mode must be chosen.
	modes := 0
	for _, on := range []bool{opts.create, opts.extract, opts.list} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		return opts, fmt.Errorf("only one of -c, -x, -t may be given")
	}
	if opts.create && len(opts.paths) == 0 {
		return opts, fmt.Errorf("-c requires at least one file or directory")
	}
	return opts, nil
}

// doCreate builds an archive from opts.paths and writes it to the -f target
// (or stdout when no -f is given).
func doCreate(opts options, stdout, stderr io.Writer) int {
	out, closeFn, err := openOutput(opts.file, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	defer closeFn()

	aw := newArchiveWriter(out, opts.verbose, stderr)
	for _, p := range opts.paths {
		if err := aw.addPath(p); err != nil {
			fmt.Fprintf(stderr, "tar: %v\n", err)
			return exitErr
		}
	}
	if err := aw.finish(); err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	return exitOK
}

// doList prints the archive's contents.
func doList(opts options, stdout, stderr io.Writer) int {
	in, closeFn, err := openInput(opts.file)
	if err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	defer closeFn()

	if err := list(in, opts.verbose, stdout); err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	return exitOK
}

// doExtract unpacks the archive into the current directory.
func doExtract(opts options, stderr io.Writer) int {
	in, closeFn, err := openInput(opts.file)
	if err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	defer closeFn()

	if err := extract(in, ".", opts.verbose, stderr); err != nil {
		fmt.Fprintf(stderr, "tar: %v\n", err)
		return exitErr
	}
	return exitOK
}

// openOutput returns the archive's destination writer. "" or "-" means stdout.
// The returned closeFn is always safe to call (no-op for stdout).
func openOutput(file string, stdout io.Writer) (io.Writer, func(), error) {
	if file == "" || file == "-" {
		return stdout, func() {}, nil
	}
	f, err := os.Create(file)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { f.Close() }, nil
}

// openInput returns the archive's source reader. "" or "-" means stdin.
func openInput(file string) (io.Reader, func(), error) {
	if file == "" || file == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { f.Close() }, nil
}

const usage = `usage:
  tar -c [-v] -f archive.tar PATH [PATH...]   create an archive
  tar -t [-v] -f archive.tar                  list archive contents
  tar -x [-v] -f archive.tar                  extract into current dir
  (omit -f or use "-" to read stdin / write stdout)`
