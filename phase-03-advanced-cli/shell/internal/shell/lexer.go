package shell

// lexer.go — Stage 1 of the shell: the TOKENIZER.
//
// Its only job is to chop a raw command line (one big string) into a flat list
// of "tokens": WORDs and the operators that separate them (| ; && || > >> < 2>).
//
// 🐍→🐹 Python analogy: this is *not* `line.split()`. A naive split would break
// `echo "hello world"` into three pieces. A real shell tokenizer understands
// quoting and escaping, so `"hello world"` stays one word. That quoting logic is
// the whole reason this file exists.

import (
	"fmt"
	"strings"
)

// tokKind enumerates every kind of token the lexer can emit.
//
// 🐍→🐹 Go has no real `enum`. The idiom is a typed integer plus a `const`
// block using `iota`, which auto-increments (tWord=0, tPipe=1, ...). Think of it
// as `class TokKind(IntEnum)` in Python.
type tokKind int

const (
	tWord     tokKind = iota // a bare word / argument, e.g. echo, "hi", file.txt
	tPipe                    // |   connect one command's stdout to the next's stdin
	tSemi                    // ;   run commands one after another (sequencing)
	tAndAnd                  // &&  run next only if previous SUCCEEDED (exit 0)
	tOrOr                    // ||  run next only if previous FAILED (exit != 0)
	tGreat                   // >   redirect stdout, truncating the file
	tDGreat                  // >>  redirect stdout, appending to the file
	tLess                    // <   redirect stdin from a file
	tErrGreat                // 2>  redirect stderr to a file
)

// wordPart is one contiguous chunk of a word, tagged with whether it is eligible
// for later variable expansion. Single-quoted and backslash-escaped chunks are
// literal (expand=false); unquoted and double-quoted chunks expand=true.
//
// WHY split a word into parts? Because `'$HOME'/"$USER"` is a SINGLE word made
// of two differently-quoted pieces: the first must stay literal, the second must
// expand. We can only honour that if we remember the quoting of each piece.
type wordPart struct {
	text   string
	expand bool
}

// Word is a sequence of parts that the parser treats as one argument.
type Word struct {
	parts []wordPart
}

// raw concatenates the parts without any expansion. Handy for syntax checks
// (like spotting an assignment `NAME=value`) before variables are resolved.
func (w *Word) raw() string {
	var b strings.Builder
	for _, p := range w.parts {
		b.WriteString(p.text)
	}
	return b.String()
}

// token is a single lexer output. Only tWord tokens carry a *Word; operator
// tokens leave word == nil.
type token struct {
	kind tokKind
	word *Word
}

// tokenize is the heart of stage 1. It scans the input byte-by-byte, building up
// the current word and emitting operator tokens when it meets them OUTSIDE of
// quotes. Returns a syntax error for unterminated quotes.
func tokenize(input string) ([]token, error) {
	var toks []token
	var cur *Word    // the word currently being built (nil = none in progress)
	started := false // true once cur exists, even if it holds an empty string ("")

	// ensure lazily creates the in-progress word. Calling it on an opening quote
	// is what makes an empty `""` still count as a real (empty) argument.
	ensure := func() {
		if cur == nil {
			cur = &Word{}
			started = true
		}
	}
	// flush finalizes the in-progress word into a token, if one exists.
	flush := func() {
		if started {
			toks = append(toks, token{kind: tWord, word: cur})
		}
		cur = nil
		started = false
	}
	addLiteral := func(s string) { ensure(); cur.parts = append(cur.parts, wordPart{s, false}) }
	// addExpand coalesces consecutive expandable characters into a single part,
	// so a run like `$MYVAR` stays whole for the expansion stage (otherwise the
	// `$` would be split away from its variable name).
	addExpand := func(s string) {
		ensure()
		if n := len(cur.parts); n > 0 && cur.parts[n-1].expand {
			cur.parts[n-1].text += s
			return
		}
		cur.parts = append(cur.parts, wordPart{s, true})
	}

	i := 0
	n := len(input)
	for i < n {
		c := input[i]
		switch {
		case c == ' ' || c == '\t':
			// Whitespace ends the current word (this is where word-splitting
			// happens) and is otherwise ignored.
			flush()
			i++

		case c == '\'':
			// SINGLE QUOTES: everything until the next ' is literal — no escapes,
			// no variable expansion. The simplest, most predictable quoting.
			ensure()
			j := i + 1
			for j < n && input[j] != '\'' {
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("syntax error: unterminated single quote")
			}
			cur.parts = append(cur.parts, wordPart{input[i+1 : j], false})
			i = j + 1

		case c == '"':
			// DOUBLE QUOTES: keep spaces literal but still allow expansion later.
			// Inside double quotes, a backslash only escapes a small set of
			// characters (" \ $ `); otherwise it stays literal.
			ensure()
			var b strings.Builder
			j := i + 1
			for j < n && input[j] != '"' {
				if input[j] == '\\' && j+1 < n {
					nx := input[j+1]
					if nx == '"' || nx == '\\' || nx == '$' || nx == '`' {
						b.WriteByte(nx)
						j += 2
						continue
					}
				}
				b.WriteByte(input[j])
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("syntax error: unterminated double quote")
			}
			cur.parts = append(cur.parts, wordPart{b.String(), true})
			i = j + 1

		case c == '\\':
			// Backslash outside quotes escapes the next single character,
			// making it literal (e.g. `a\ b` is the one word "a b").
			if i+1 < n {
				addLiteral(string(input[i+1]))
				i += 2
			} else {
				i++
			}

		case c == '|':
			flush()
			if i+1 < n && input[i+1] == '|' {
				toks = append(toks, token{kind: tOrOr})
				i += 2
			} else {
				toks = append(toks, token{kind: tPipe})
				i++
			}

		case c == '&':
			// We only support `&&`. A lone `&` (background jobs) is out of scope.
			if i+1 < n && input[i+1] == '&' {
				flush()
				toks = append(toks, token{kind: tAndAnd})
				i += 2
			} else {
				return nil, fmt.Errorf("syntax error: background '&' not supported")
			}

		case c == ';':
			flush()
			toks = append(toks, token{kind: tSemi})
			i++

		case c == '>':
			// A bare word "2" immediately before '>' means "redirect stderr"
			// (the classic `2>` / `2>>` form). Otherwise it's stdout redirection.
			if started && len(cur.parts) == 1 && cur.raw() == "2" {
				cur = nil
				started = false
				toks = append(toks, token{kind: tErrGreat})
				i++ // consume the '>'
				if i < n && input[i] == '>' {
					i++ // 2>> behaves like 2> (overwrite) in this shell
				}
			} else {
				flush()
				if i+1 < n && input[i+1] == '>' {
					toks = append(toks, token{kind: tDGreat})
					i += 2
				} else {
					toks = append(toks, token{kind: tGreat})
					i++
				}
			}

		case c == '<':
			flush()
			toks = append(toks, token{kind: tLess})
			i++

		default:
			// Any ordinary character extends the current (expandable) word.
			addExpand(string(c))
			i++
		}
	}
	flush()
	return toks, nil
}
