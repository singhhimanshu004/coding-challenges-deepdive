package shell

// parser.go — Stage 2 of the shell: the PARSER.
//
// The lexer gave us a *flat* list of tokens. The parser turns that flat list
// into a small tree (an AST — Abstract Syntax Tree) that mirrors how a shell
// actually thinks about a command line:
//
//	List      ::= AndOr (';' AndOr)*          // ; sequences independent commands
//	AndOr     ::= Pipeline (('&&'|'||') Pipeline)*
//	Pipeline  ::= Command ('|' Command)*      // | streams stdout -> next stdin
//	Command   ::= (Word | Redirection)+       // a program + its args + redirects
//
// 🐍→🐹 If you've ever written a recursive-descent parser in Python, this is the
// same shape: one method per grammar rule, each consuming tokens and calling the
// rule "below" it. The grammar's nesting order encodes operator precedence:
// `;` is loosest, then `&&`/`||`, then `|`, then redirections bind tightest.

import "fmt"

// ---- AST node types -------------------------------------------------------

// Redirect captures a single redirection on a command.
//
//	fd:   which file descriptor — 0=stdin, 1=stdout, 2=stderr
//	mode: "in" (<), "out" (> truncate), or "append" (>>)
//	word: the filename (still un-expanded; resolved at execution time)
type Redirect struct {
	fd   int
	mode string
	word *Word
}

// Command is the leaf of the tree: one program invocation with its arguments
// and any redirections attached to it.
type Command struct {
	Args   []*Word
	Redirs []*Redirect
}

// Pipeline is one or more Commands joined by '|'. Every stage runs concurrently;
// stage k's stdout is connected to stage k+1's stdin.
type Pipeline struct {
	Cmds []*Command
}

// AndOr is a chain of Pipelines joined by && / ||. Ops has length len(Pipelines)-1
// and records which operator sits between each adjacent pair.
type AndOr struct {
	Pipelines []*Pipeline
	Ops       []tokKind // each entry is tAndAnd or tOrOr
}

// List is the whole command line: AndOr chains separated by ';'.
type List struct {
	Items []*AndOr
}

// ---- the parser -----------------------------------------------------------

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() (token, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return token{}, false
}

func (p *parser) next() token {
	t := p.toks[p.pos]
	p.pos++
	return t
}

func parse(toks []token) (*List, error) {
	p := &parser{toks: toks}
	list, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("syntax error: unexpected token")
	}
	return list, nil
}

func (p *parser) parseList() (*List, error) {
	list := &List{}
	for {
		// Allow and skip leading/duplicate/trailing ';'.
		if t, ok := p.peek(); ok && t.kind == tSemi {
			p.next()
			continue
		}
		if _, ok := p.peek(); !ok {
			break
		}
		ao, err := p.parseAndOr()
		if err != nil {
			return nil, err
		}
		list.Items = append(list.Items, ao)
	}
	return list, nil
}

func (p *parser) parseAndOr() (*AndOr, error) {
	first, err := p.parsePipeline()
	if err != nil {
		return nil, err
	}
	ao := &AndOr{Pipelines: []*Pipeline{first}}
	for {
		t, ok := p.peek()
		if !ok || (t.kind != tAndAnd && t.kind != tOrOr) {
			break
		}
		p.next()
		nextPipe, err := p.parsePipeline()
		if err != nil {
			return nil, err
		}
		ao.Pipelines = append(ao.Pipelines, nextPipe)
		ao.Ops = append(ao.Ops, t.kind)
	}
	return ao, nil
}

func (p *parser) parsePipeline() (*Pipeline, error) {
	first, err := p.parseCommand()
	if err != nil {
		return nil, err
	}
	pl := &Pipeline{Cmds: []*Command{first}}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tPipe {
			break
		}
		p.next()
		c, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		pl.Cmds = append(pl.Cmds, c)
	}
	return pl, nil
}

// parseCommand consumes words and redirections until it hits a token that
// belongs to a higher grammar rule (a pipe, semicolon, or logical operator) or
// runs out of input.
func (p *parser) parseCommand() (*Command, error) {
	cmd := &Command{}
	for {
		t, ok := p.peek()
		if !ok {
			break
		}
		switch t.kind {
		case tWord:
			p.next()
			cmd.Args = append(cmd.Args, t.word)

		case tGreat, tDGreat, tLess, tErrGreat:
			p.next()
			// A redirection operator must be followed by a word (the target file).
			target, ok := p.peek()
			if !ok || target.kind != tWord {
				return nil, fmt.Errorf("syntax error: expected filename after redirection")
			}
			p.next()
			r := &Redirect{word: target.word}
			switch t.kind {
			case tGreat:
				r.fd, r.mode = 1, "out"
			case tDGreat:
				r.fd, r.mode = 1, "append"
			case tLess:
				r.fd, r.mode = 0, "in"
			case tErrGreat:
				r.fd, r.mode = 2, "out"
			}
			cmd.Redirs = append(cmd.Redirs, r)

		default:
			// tPipe / tSemi / tAndAnd / tOrOr — not ours to consume.
			goto done
		}
	}
done:
	if len(cmd.Args) == 0 && len(cmd.Redirs) == 0 {
		return nil, fmt.Errorf("syntax error: empty command")
	}
	return cmd, nil
}
