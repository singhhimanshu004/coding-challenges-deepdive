package shell

// executor.go — Stage 3 of the shell: EXECUTION.
//
// This is where the AST becomes running processes. It is the file that finally
// answers "what does `cmd1 | cmd2 > file` actually do under the hood?".
//
// Top-down structure mirrors the grammar:
//
//	execList     — run each ';'-separated AndOr in order
//	execAndOr    — apply && / || short-circuit logic
//	execPipeline — the interesting part: wire pipes & redirects, fork/exec
//	  execSimple — fast path for a single command (lets `cd` mutate the shell)
//	  execMulti  — the general N-stage pipeline

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// execList runs ';'-sequenced items one after another, stopping early if a
// builtin asked us to exit.
func (sh *Shell) execList(l *List) int {
	for _, ao := range l.Items {
		sh.execAndOr(ao)
		if sh.shouldExit {
			break
		}
	}
	return sh.lastStatus
}

// execAndOr implements the short-circuit semantics of && and ||:
//   - after `&&`, run the next pipeline only if the previous one SUCCEEDED (0)
//   - after `||`, run the next pipeline only if the previous one FAILED (!=0)
func (sh *Shell) execAndOr(ao *AndOr) int {
	status := sh.execPipeline(ao.Pipelines[0])
	for idx, op := range ao.Ops {
		if op == tAndAnd && status != 0 {
			continue // previous failed; skip this &&-guarded stage
		}
		if op == tOrOr && status == 0 {
			continue // previous succeeded; skip this ||-guarded stage
		}
		status = sh.execPipeline(ao.Pipelines[idx+1])
		if sh.shouldExit {
			break
		}
	}
	return status
}

func (sh *Shell) execPipeline(p *Pipeline) int {
	if len(p.Cmds) == 1 {
		return sh.execSimple(p.Cmds[0])
	}
	return sh.execMulti(p)
}

var assignRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// execSimple handles a single command (no pipe). Keeping this separate from the
// multi-stage path matters: a lone builtin like `cd` must run directly in the
// shell goroutine so it can mutate shell state.
func (sh *Shell) execSimple(cmd *Command) int {
	// --- Variable assignment: `NAME=value` with no command word. ----------
	// e.g. `GREETING=hi`. These set shell-local variables and run nothing.
	if len(cmd.Args) > 0 {
		allAssign := true
		for _, w := range cmd.Args {
			if !assignRe.MatchString(w.raw()) {
				allAssign = false
				break
			}
		}
		if allAssign {
			for _, w := range cmd.Args {
				s := sh.expandWord(w)
				eq := strings.IndexByte(s, '=')
				sh.setVar(s[:eq], s[eq+1:])
			}
			sh.lastStatus = 0
			return 0
		}
	}

	args := sh.expandArgs(cmd)

	in, out, errw, closers, err := sh.applyRedirs(cmd.Redirs, sh.In, sh.Out, sh.Err)
	defer closeAll(closers)
	if err != nil {
		fmt.Fprintf(sh.Err, "gosh: %v\n", err)
		sh.lastStatus = 1
		return 1
	}

	// A redirection with no command (e.g. `> file`) is valid: it just creates/
	// truncates the file. The open already happened above, so we're done.
	if len(args) == 0 {
		sh.lastStatus = 0
		return 0
	}

	// Builtin? Run it in-process so it can change directory, env, etc.
	if bfn, ok := builtins[args[0]]; ok {
		st := bfn(sh, args, in, out, errw)
		sh.lastStatus = st
		return st
	}

	// External command: fork/exec a child. os/exec resolves args[0] on $PATH.
	ec := exec.Command(args[0], args[1:]...)
	ec.Stdin, ec.Stdout, ec.Stderr = in, out, errw
	st := sh.runChild(ec, args[0])
	sh.lastStatus = st
	return st
}

// execMulti wires up an N-stage pipeline. THIS is the core systems lesson.
//
// For `a | b | c` we create two anonymous pipes and connect them:
//
//	          pipe0                 pipe1
//	a.stdout --> [w0] === [r0] --> b.stdin
//	                      b.stdout --> [w1] === [r1] --> c.stdin
//
// Every stage starts CONCURRENTLY. Data streams through the kernel pipe buffers,
// so `yes | head` works without `yes` ever finishing. The trickiest detail is
// fd bookkeeping: the PARENT must close its copies of the pipe ends, otherwise a
// reader never sees EOF and the pipeline hangs forever.
func (sh *Shell) execMulti(p *Pipeline) int {
	n := len(p.Cmds)

	// Create the n-1 connecting pipes up front.
	type pipePair struct{ r, w *os.File }
	pipes := make([]pipePair, n-1)
	for i := 0; i < n-1; i++ {
		r, w, err := os.Pipe()
		if err != nil {
			fmt.Fprintf(sh.Err, "gosh: pipe: %v\n", err)
			sh.lastStatus = 1
			return 1
		}
		pipes[i] = pipePair{r, w}
	}

	statuses := make([]int, n)
	var wg sync.WaitGroup
	var parentCloses []io.Closer // pipe ends owned by external stages

	for i, cmd := range p.Cmds {
		// Pick this stage's pipe ends. Stage i reads from pipe i-1 and writes to
		// pipe i (except the first reads real stdin, the last writes real stdout).
		var origIn, origOut *os.File
		baseIn := sh.In
		baseOut := sh.Out
		if i > 0 {
			origIn = pipes[i-1].r
			baseIn = origIn
		}
		if i < n-1 {
			origOut = pipes[i].w
			baseOut = origOut
		}

		// Per-command redirections override the pipe wiring (e.g. last stage
		// `> file` sends stdout to the file instead of the terminal).
		in, out, errw, closers, err := sh.applyRedirs(cmd.Redirs, baseIn, baseOut, sh.Err)
		if err != nil {
			fmt.Fprintf(sh.Err, "gosh: %v\n", err)
			statuses[i] = 1
			closeAll(closers)
			// Still ensure this stage's pipe ends get closed below.
		}

		args := sh.expandArgs(cmd)

		// Ownership rule (see function doc): each pipe end is used by exactly one
		// stage. Whoever owns it must close it so EOF propagates.
		//   - external stage: the PARENT closes after Start (child has a dup)
		//   - builtin stage:  the goroutine closes when it finishes
		ownEnds := []*os.File{}
		if origIn != nil {
			ownEnds = append(ownEnds, origIn)
		}
		if origOut != nil {
			ownEnds = append(ownEnds, origOut)
		}

		if bfn, ok := builtins[args[0]]; ok && err == nil {
			// Builtin pipeline stage runs in its own goroutine.
			wg.Add(1)
			go func(i int, bfn builtinFunc, args []string, in io.Reader, out, errw io.Writer, ownEnds []*os.File, closers []io.Closer) {
				defer wg.Done()
				statuses[i] = bfn(sh, args, in, out, errw)
				for _, f := range ownEnds {
					f.Close()
				}
				closeAll(closers)
			}(i, bfn, args, in, out, errw, ownEnds, closers)
			continue
		}

		if err != nil {
			// Redirection failed for an external stage: skip exec, but still
			// release our pipe ends so the rest of the pipeline can drain.
			for _, f := range ownEnds {
				parentCloses = append(parentCloses, f)
			}
			continue
		}

		ec := exec.Command(args[0], args[1:]...)
		ec.Stdin, ec.Stdout, ec.Stderr = in, out, errw
		if startErr := ec.Start(); startErr != nil {
			fmt.Fprintf(sh.Err, "gosh: %s: command not found\n", args[0])
			statuses[i] = 127
		} else {
			wg.Add(1)
			go func(i int, ec *exec.Cmd) {
				defer wg.Done()
				statuses[i] = exitCode(ec.Wait())
			}(i, ec)
		}
		// Parent owns these pipe ends + redirect files for an external stage.
		for _, f := range ownEnds {
			parentCloses = append(parentCloses, f)
		}
		parentCloses = append(parentCloses, closers...)
	}

	// Critical: close the parent's pipe copies NOW so readers can reach EOF.
	closeAll(parentCloses)
	wg.Wait()

	sh.lastStatus = statuses[n-1] // a pipeline's status is its LAST stage
	return sh.lastStatus
}

// runChild runs an already-configured external command synchronously and maps
// the result to an exit code, printing a "command not found" message when the
// program can't be located on $PATH.
func (sh *Shell) runChild(ec *exec.Cmd, name string) int {
	err := ec.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	fmt.Fprintf(sh.Err, "gosh: %s: command not found\n", name)
	return 127
}

// applyRedirs opens the files named by a command's redirections and returns the
// resulting (in, out, err) streams plus the closers to release afterwards. It
// starts from the supplied base streams so it composes with pipeline wiring.
func (sh *Shell) applyRedirs(rs []*Redirect, baseIn io.Reader, baseOut, baseErr io.Writer) (io.Reader, io.Writer, io.Writer, []io.Closer, error) {
	in, out, errw := baseIn, baseOut, baseErr
	var closers []io.Closer
	for _, r := range rs {
		name := sh.expandWord(r.word)
		switch r.mode {
		case "in":
			f, err := os.Open(name)
			if err != nil {
				return in, out, errw, closers, err
			}
			closers = append(closers, f)
			in = f
		case "out":
			f, err := os.Create(name) // truncate or create
			if err != nil {
				return in, out, errw, closers, err
			}
			closers = append(closers, f)
			if r.fd == 2 {
				errw = f
			} else {
				out = f
			}
		case "append":
			f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return in, out, errw, closers, err
			}
			closers = append(closers, f)
			if r.fd == 2 {
				errw = f
			} else {
				out = f
			}
		}
	}
	return in, out, errw, closers, nil
}

func closeAll(cs []io.Closer) {
	for _, c := range cs {
		c.Close()
	}
}

// exitCode extracts a numeric exit status from the error returned by Cmd.Wait.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
