package sed

import (
	"fmt"
	"regexp"
	"strings"
)

// Parse turns a sed script string into an ordered list of commands.
//
// A script is one or more commands separated by `;` or newlines, e.g.
// `1,3d`, `s/foo/bar/g`, or the multi-command `s/a/b/; 2p`. This is the
// "front end" of our interpreter: lexing + parsing the mini command language
// into the data model defined in command.go.
func Parse(script string) ([]*command, error) {
	p := &parser{src: script}
	var cmds []*command
	for {
		p.skipSeparators()
		if p.eof() {
			break
		}
		c, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, nil
}

// parser is a tiny hand-written recursive-descent parser. It walks `src` with a
// single cursor `pos`. Hand-rolling it (rather than reaching for a parser
// library) keeps the whole command language visible in one file — the point of
// the exercise.
type parser struct {
	src string
	pos int
}

func (p *parser) eof() bool  { return p.pos >= len(p.src) }
func (p *parser) peek() byte { return p.src[p.pos] }

// next returns the current byte and advances the cursor.
func (p *parser) next() byte {
	c := p.src[p.pos]
	p.pos++
	return c
}

func (p *parser) skipSpaces() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

// skipSeparators eats the whitespace and command separators (`;`, newlines)
// that sit between commands.
func (p *parser) skipSeparators() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\n', '\r', ';':
			p.pos++
		default:
			return
		}
	}
}

// parseCommand parses one [address[,address]] verb [args] unit.
func (p *parser) parseCommand() (*command, error) {
	c := &command{}

	a1, err := p.parseAddress()
	if err != nil {
		return nil, err
	}
	if a1 != nil {
		c.a1 = a1
		p.skipSpaces()
		if !p.eof() && p.peek() == ',' {
			p.pos++ // consume ','
			p.skipSpaces()
			a2, err := p.parseAddress()
			if err != nil {
				return nil, err
			}
			if a2 == nil {
				return nil, fmt.Errorf("expected a second address after ','")
			}
			c.a2 = a2
		}
	}

	p.skipSpaces()
	if p.eof() {
		return nil, fmt.Errorf("missing command after address")
	}

	switch ch := p.next(); ch {
	case 's':
		if err := p.parseSubstitute(c); err != nil {
			return nil, err
		}
	case 'p':
		c.kind = 'p'
	case 'd':
		c.kind = 'd'
	default:
		return nil, fmt.Errorf("unknown command: %q", string(ch))
	}
	return c, nil
}

// parseAddress parses a single address, or returns (nil, nil) when the cursor
// is not sitting on one (meaning the command has no address).
func (p *parser) parseAddress() (*address, error) {
	p.skipSpaces()
	if p.eof() {
		return nil, nil
	}
	switch ch := p.peek(); {
	case ch == '$':
		p.pos++
		return &address{kind: addrLast}, nil

	case ch >= '0' && ch <= '9':
		n := 0
		for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
			n = n*10 + int(p.next()-'0')
		}
		return &address{kind: addrLine, line: n}, nil

	case ch == '/':
		p.pos++ // consume opening '/'
		pat, err := p.readUntilDelim('/')
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid address regex %q: %w", pat, err)
		}
		return &address{kind: addrRegex, re: re}, nil
	}
	return nil, nil
}

// parseSubstitute parses the body of an `s` command: s<delim>regex<delim>repl<delim>[flags].
//
// The delimiter is whatever character follows the `s` (usually `/`, but sed
// lets you pick any char, which is handy when the pattern itself contains
// slashes, e.g. `s|/usr|/opt|`).
func (p *parser) parseSubstitute(c *command) error {
	if p.eof() {
		return fmt.Errorf("incomplete s command")
	}
	delim := p.next()
	if delim == '\n' || delim == ';' {
		return fmt.Errorf("invalid s delimiter")
	}

	pattern, err := p.readUntilDelim(delim)
	if err != nil {
		return err
	}
	replacement, err := p.readUntilDelim(delim)
	if err != nil {
		return err
	}

	// Flags: g (global), i (case-insensitive), p (print on change). They run
	// together with no separator, e.g. `gi`.
	var global, ignoreCase, printFlag bool
	for !p.eof() {
		switch p.peek() {
		case 'g':
			global = true
		case 'i':
			ignoreCase = true
		case 'p':
			printFlag = true
		default:
			goto done
		}
		p.pos++
	}
done:

	src := pattern
	if ignoreCase {
		// `(?i)` is Go's inline flag for case-insensitive matching — the same
		// effect sed gets from the trailing `i`.
		src = "(?i)" + src
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return fmt.Errorf("invalid s/// regex %q: %w", pattern, err)
	}

	c.kind = 's'
	c.re = re
	c.global = global
	c.printFlag = printFlag
	c.replTemplate = convertReplacement(replacement)
	return nil
}

// readUntilDelim consumes characters up to (and consuming) the next unescaped
// `delim`. A backslash escapes the delimiter (`\/` becomes a literal `/`); any
// other backslash sequence is passed through untouched so the regex engine —
// or, for replacements, convertReplacement — can interpret it (`\.`, `\1`, …).
func (p *parser) readUntilDelim(delim byte) (string, error) {
	var b strings.Builder
	for !p.eof() {
		c := p.next()
		if c == '\\' && !p.eof() {
			n := p.next()
			if n == delim {
				b.WriteByte(delim) // \<delim> -> literal delimiter
			} else {
				b.WriteByte('\\') // keep the escape intact for the next stage
				b.WriteByte(n)
			}
			continue
		}
		if c == delim {
			return b.String(), nil
		}
		b.WriteByte(c)
	}
	return "", fmt.Errorf("unterminated expression: expected delimiter %q", string(delim))
}

// convertReplacement rewrites a sed replacement string into the form Go's
// regexp.ExpandString understands.
//
// sed and Go speak different replacement dialects:
//
//	sed:  \1 \2 …  capture-group backreference   &  whole match
//	Go:   ${1} …                                 $0 …
//
// So we translate \N → ${N}, & → ${0}, escape any literal `$` as `$$` (since
// `$` is special to Go), and resolve the usual backslash escapes (\n, \t, \&,
// \\). This is the one place the two dialects meet.
func convertReplacement(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			if i+1 >= len(s) {
				b.WriteByte('\\')
				break
			}
			n := s[i+1]
			i++
			switch {
			case n >= '0' && n <= '9':
				b.WriteString("${")
				b.WriteByte(n)
				b.WriteByte('}')
			case n == 'n':
				b.WriteByte('\n')
			case n == 't':
				b.WriteByte('\t')
			case n == '&':
				b.WriteByte('&') // literal ampersand
			case n == '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte(n)
			}
		case '&':
			b.WriteString("${0}") // whole match
		case '$':
			b.WriteString("$$") // escape: a literal $ in the output
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
