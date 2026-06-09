package shell

// repl.go — the interactive Read-Eval-Print Loop and the non-interactive script
// runners. These are the only parts that touch real terminals/signals, which is
// why the tests drive RunLine directly and never need a TTY.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

// RunInteractive runs the classic prompt loop until EOF (Ctrl-D) or `exit`.
//
// 🔔 Signals: we install a handler for SIGINT (Ctrl-C). Because the shell and
// its foreground child live in the same process group, BOTH receive the signal
// when you press Ctrl-C. The default action kills the child; our handler simply
// swallows the signal in the shell so the prompt survives instead of the whole
// shell dying. That is exactly the behaviour you want: Ctrl-C interrupts the
// running command, not your session.
func (sh *Shell) RunInteractive() int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			// Swallow SIGINT at the shell level. The child (if any) already got
			// its own copy and will terminate; we just print a fresh line.
			fmt.Fprint(sh.Out, "\n"+sh.prompt())
		}
	}()

	reader := bufio.NewReader(sh.In)
	for {
		fmt.Fprint(sh.Out, sh.prompt())
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			sh.RunLine(strings.TrimRight(line, "\n"))
		}
		if sh.shouldExit {
			break
		}
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(sh.Out) // tidy newline after Ctrl-D
			}
			break
		}
	}
	return sh.lastStatus
}

// RunReader executes a script supplied as a stream (e.g. a file), line by line.
func (sh *Shell) RunReader(r io.Reader) int {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue // skip blanks and comments
		}
		sh.RunLine(line)
		if sh.shouldExit {
			break
		}
	}
	return sh.lastStatus
}

// prompt renders the shell prompt: "gosh <dir>$ ", where <dir> is the basename
// of the current working directory (or ~ for HOME).
func (sh *Shell) prompt() string {
	wd, err := os.Getwd()
	if err != nil {
		return "gosh$ "
	}
	if home := os.Getenv("HOME"); home != "" && wd == home {
		return "gosh ~$ "
	}
	return fmt.Sprintf("gosh %s$ ", filepath.Base(wd))
}
