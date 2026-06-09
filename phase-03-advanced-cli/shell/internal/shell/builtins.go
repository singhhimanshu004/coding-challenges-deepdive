package shell

// builtins.go — commands that the shell runs *in its own process* instead of
// fork/exec-ing a child.
//
// 🎯 WHY must some commands be builtins? The headline example is `cd`.
// A child process has its OWN copy of the working directory. If `cd` were an
// external program, it would change *its* directory and then exit — the parent
// shell (you) would be left exactly where you started. The directory change has
// to happen INSIDE the shell process, so `cd` MUST be a builtin. The same logic
// applies to anything that mutates shell state: `exit`, `export`, variable
// assignment, etc. They change the shell itself, so they cannot be a child.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// builtinFunc is the signature every builtin shares. It gets the shell (for
// state), the already-expanded args, and the three streams (so builtins respect
// redirections and can run as a stage inside a pipeline).
type builtinFunc func(sh *Shell, args []string, in io.Reader, out, errw io.Writer) int

// builtins is the dispatch table. Looking a name up here is how the executor
// decides "run in-process" vs "fork/exec a child".
//
// 🐍→🐹 It is filled in init() rather than as a literal because biType calls
// isBuiltin, which reads this map — a static initializer literal would create a
// compile-time initialization cycle. init() defers population to runtime.
var builtins map[string]builtinFunc

func init() {
	builtins = map[string]builtinFunc{
		"cd":     biCd,
		"pwd":    biPwd,
		"exit":   biExit,
		"echo":   biEcho,
		"export": biExport,
		"type":   biType,
	}
}

func isBuiltin(name string) bool {
	_, ok := builtins[name]
	return ok
}

// biCd changes the shell's working directory. With no argument it goes to $HOME;
// `cd -` returns to the previous directory ($OLDPWD).
func biCd(sh *Shell, args []string, _ io.Reader, out, errw io.Writer) int {
	var dir string
	switch {
	case len(args) > 1 && args[1] == "-":
		dir = sh.getVar("OLDPWD")
		fmt.Fprintln(out, dir) // bash prints the target dir for `cd -`
	case len(args) > 1:
		dir = args[1]
	default:
		dir = sh.getVar("HOME")
	}
	if dir == "" {
		fmt.Fprintln(errw, "cd: HOME not set")
		return 1
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintf(errw, "cd: %s: %v\n", dir, err)
		return 1
	}
	now, _ := os.Getwd()
	sh.cwd = now
	sh.setVar("OLDPWD", old)
	sh.setVar("PWD", now)
	os.Setenv("PWD", now) // keep the exported PWD in sync for child processes
	return 0
}

// biPwd prints the current working directory.
func biPwd(_ *Shell, _ []string, _ io.Reader, out, _ io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		return 1
	}
	fmt.Fprintln(out, wd)
	return 0
}

// biExit asks the REPL to stop. With an argument it uses that as the exit code;
// otherwise it reuses the last command's status.
func biExit(sh *Shell, args []string, _ io.Reader, _, errw io.Writer) int {
	code := sh.lastStatus
	if len(args) > 1 {
		if n, err := strconv.Atoi(args[1]); err == nil {
			code = n
		} else {
			fmt.Fprintf(errw, "exit: %s: numeric argument required\n", args[1])
			code = 2
		}
	}
	sh.shouldExit = true
	sh.lastStatus = code
	return code
}

// biEcho writes its arguments separated by spaces, followed by a newline unless
// the -n flag suppresses it.
func biEcho(_ *Shell, args []string, _ io.Reader, out, _ io.Writer) int {
	i := 1
	newline := true
	if i < len(args) && args[i] == "-n" {
		newline = false
		i++
	}
	fmt.Fprint(out, strings.Join(args[i:], " "))
	if newline {
		fmt.Fprintln(out)
	}
	return 0
}

// biExport promotes shell variables into the process environment so that child
// processes inherit them. `export NAME=value` sets-and-exports in one step;
// `export NAME` exports an already-set shell variable; bare `export` lists the
// current environment.
func biExport(sh *Shell, args []string, _ io.Reader, out, _ io.Writer) int {
	if len(args) == 1 {
		for _, kv := range os.Environ() {
			fmt.Fprintf(out, "export %s\n", kv)
		}
		return 0
	}
	for _, a := range args[1:] {
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			name, val := a[:eq], a[eq+1:]
			os.Setenv(name, val)
			delete(sh.vars, name) // now lives in the environment, not shell-local
		} else if v, ok := sh.vars[a]; ok {
			os.Setenv(a, v)
			delete(sh.vars, a)
		}
	}
	return 0
}

// biType reports how a name would be resolved: as a builtin, or as a path found
// on $PATH. This mirrors bash's `type` and is a great teaching tool for PATH
// resolution.
func biType(_ *Shell, args []string, _ io.Reader, out, _ io.Writer) int {
	status := 0
	for _, name := range args[1:] {
		switch {
		case isBuiltin(name):
			fmt.Fprintf(out, "%s is a shell builtin\n", name)
		default:
			if p, err := exec.LookPath(name); err == nil {
				fmt.Fprintf(out, "%s is %s\n", name, p)
			} else {
				fmt.Fprintf(out, "%s: not found\n", name)
				status = 1
			}
		}
	}
	return status
}
