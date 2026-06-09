package shell

// expand.go — variable expansion: turning $VAR, ${VAR} and $? into their values.
//
// Expansion happens AFTER tokenizing/parsing but BEFORE execution, and only on
// word-parts that were tagged expand=true by the lexer (i.e. not single-quoted
// and not backslash-escaped). This is why `'$HOME'` prints literally while
// `"$HOME"` and `$HOME` print your home directory.

import (
	"os"
	"strconv"
	"strings"
)

// getVar looks a variable up: shell-local variables first, then the process
// environment (which is where exported variables live).
func (sh *Shell) getVar(name string) string {
	if v, ok := sh.vars[name]; ok {
		return v
	}
	return os.Getenv(name)
}

// setVar stores a shell-local variable (not visible to child processes until it
// is `export`ed).
func (sh *Shell) setVar(name, value string) { sh.vars[name] = value }

func isNameStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
func isNameChar(b byte) bool {
	return isNameStart(b) || (b >= '0' && b <= '9')
}

// expandWord resolves every expandable part of a word into a final string.
func (sh *Shell) expandWord(w *Word) string {
	var b strings.Builder
	for _, p := range w.parts {
		if p.expand {
			b.WriteString(sh.expandStr(p.text))
		} else {
			b.WriteString(p.text) // literal: single-quoted or escaped
		}
	}
	return b.String()
}

// expandStr scans one string for $-expansions. Supported forms:
//
//	$?        -> last command's exit status
//	$$        -> the shell's process id
//	$NAME     -> value of NAME (name = [A-Za-z_][A-Za-z0-9_]*)
//	${NAME}   -> same, with explicit braces so it can abut other text
//
// Anything that doesn't match (e.g. a lone `$` or `$1`) is left untouched.
func (sh *Shell) expandStr(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) {
			nx := s[i+1]
			switch {
			case nx == '?':
				b.WriteString(strconv.Itoa(sh.lastStatus))
				i += 2
				continue
			case nx == '$':
				b.WriteString(strconv.Itoa(os.Getpid()))
				i += 2
				continue
			case nx == '{':
				j := i + 2
				for j < len(s) && s[j] != '}' {
					j++
				}
				if j < len(s) { // found closing brace
					b.WriteString(sh.getVar(s[i+2 : j]))
					i = j + 1
					continue
				}
			case isNameStart(nx):
				j := i + 1
				for j < len(s) && isNameChar(s[j]) {
					j++
				}
				b.WriteString(sh.getVar(s[i+1 : j]))
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// expandArgs resolves all of a command's argument words into plain strings,
// ready to hand to exec or a builtin.
func (sh *Shell) expandArgs(cmd *Command) []string {
	args := make([]string, 0, len(cmd.Args))
	for _, w := range cmd.Args {
		args = append(args, sh.expandWord(w))
	}
	return args
}
