package main

// parser.go — the *filter language* parser. It turns the flat token slice from
// the lexer into an Abstract Syntax Tree (AST): a tree of nodes that mirrors the
// structure of the filter. The evaluator (eval.go) then walks that tree.
//
// 🐍 Python analogy: this is exactly what Python's `ast.parse` does — text in,
// tree out. Each AST node here is a tiny Go struct implementing the `Node`
// interface, the way Python's `ast` module has one class per syntax construct.
//
// ── The grammar we implement (lowest precedence first) ──────────────────────
//
//	pipe     := comma ( '|' comma )*
//	comma    := compare ( ',' compare )*
//	compare  := additive ( ('=='|'!='|'<'|'>'|'<='|'>=') additive )?
//	additive := multiply ( ('+'|'-') multiply )*
//	multiply := postfix  ( ('*'|'/') postfix )*
//	postfix  := primary ( '.'IDENT | '['expr?']' | '?' )*
//	primary  := '.' IDENT?          (field access or identity)
//	          | '[' pipe? ']'        (array construction)
//	          | '(' pipe ')'         (grouping)
//	          | NUMBER | STRING      (literals)
//	          | IDENT ( '(' args ')' )?   (builtin / function call)
//
// Lower-precedence rules sit *above* higher-precedence ones, so `|` binds the
// loosest and a path suffix like `.foo` binds the tightest — just like real jq.

import "fmt"

// Node is the interface every AST node implements. It is intentionally empty of
// behaviour: the evaluator type-switches over concrete node types. (An
// alternative OO design would put an `Eval` method on each node; we keep eval
// logic in one file for teaching clarity.)
//
// 🐍 Python analogy: like a common base class `ast.AST` that every node extends.
type Node interface{ node() }

// The concrete AST node types. Each `node()` method is a no-op "marker" that
// makes the type satisfy the Node interface.
type (
	// Identity is bare `.` — return the input unchanged.
	Identity struct{}

	// Field is `.name` — look up a key on the input object.
	Field struct{ Name string }

	// Index is `.[n]` / `.["k"]` — index into an array or object.
	Index struct{ Index Node }

	// Iterate is `.[]` — stream every element of an array / value of an object.
	Iterate struct{}

	// Pipe feeds every output of Left into Right as Right's input (the `|`).
	Pipe struct{ Left, Right Node }

	// Comma runs Left and Right on the same input and concatenates their
	// output streams (the `,`).
	Comma struct{ Left, Right Node }

	// ArrayConstruct is `[ expr ]` — collect *all* outputs of expr into one
	// array value. `[]` (empty) builds an empty array.
	ArrayConstruct struct{ Expr Node }

	// Literal is a constant number / string / bool / null appearing in the filter.
	Literal struct{ Value any }

	// Binary is an arithmetic or comparison operation.
	Binary struct {
		Op          tokenType
		Left, Right Node
	}

	// Call is a builtin invocation: length, keys, values, has(k), select(f), map(f).
	Call struct {
		Name string
		Args []Node
	}

	// Try is `expr?` — run expr but swallow any error, producing no output
	// instead of failing.
	Try struct{ Expr Node }
)

func (Identity) node()       {}
func (Field) node()          {}
func (Index) node()          {}
func (Iterate) node()        {}
func (Pipe) node()           {}
func (Comma) node()          {}
func (ArrayConstruct) node() {}
func (Literal) node()        {}
func (Binary) node()         {}
func (Call) node()           {}
func (Try) node()            {}

// parser holds the token slice and a cursor. Same single-struct-of-state idiom
// as the JSON parser.
type parser struct {
	toks []token
	pos  int
}

// ParseFilter is the public entry point: lex then parse a filter string into an
// AST root node.
func ParseFilter(input string) (Node, error) {
	toks, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	node, err := p.parsePipe()
	if err != nil {
		return nil, err
	}
	if p.cur().typ != tEOF {
		return nil, fmt.Errorf("unexpected token %q at position %d", p.cur().val, p.cur().pos)
	}
	return node, nil
}

// --- cursor helpers --------------------------------------------------------

func (p *parser) cur() token { return p.toks[p.pos] }
func (p *parser) peek() token { // look one token ahead without consuming
	if p.pos+1 < len(p.toks) {
		return p.toks[p.pos+1]
	}
	return p.toks[len(p.toks)-1] // tEOF
}
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// expect consumes the current token if it is of type typ, else errors.
func (p *parser) expect(typ tokenType, what string) (token, error) {
	if p.cur().typ != typ {
		return token{}, fmt.Errorf("expected %s at position %d, got %q", what, p.cur().pos, p.cur().val)
	}
	return p.advance(), nil
}

// --- grammar rules (one method per precedence level) -----------------------

func (p *parser) parsePipe() (Node, error) {
	left, err := p.parseComma()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tPipe {
		p.advance()
		right, err := p.parseComma()
		if err != nil {
			return nil, err
		}
		left = Pipe{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseComma() (Node, error) {
	left, err := p.parseCompare()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tComma {
		p.advance()
		right, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		left = Comma{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseCompare() (Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	switch p.cur().typ {
	case tEq, tNe, tLt, tGt, tLe, tGe:
		op := p.advance().typ
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		return Binary{Op: op, Left: left, Right: right}, nil
	}
	return left, nil
}

func (p *parser) parseAdditive() (Node, error) {
	left, err := p.parseMultiply()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tPlus || p.cur().typ == tMinus {
		op := p.advance().typ
		right, err := p.parseMultiply()
		if err != nil {
			return nil, err
		}
		left = Binary{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseMultiply() (Node, error) {
	left, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	for p.cur().typ == tStar || p.cur().typ == tSlash {
		op := p.advance().typ
		right, err := p.parsePostfix()
		if err != nil {
			return nil, err
		}
		left = Binary{Op: op, Left: left, Right: right}
	}
	return left, nil
}

// parsePostfix handles chaining that binds tightest: `.foo.bar`, `keys[0]`,
// `.items[]`, and the optional `?` suffix. It builds each suffix as a Pipe so
// the evaluator only ever has to understand "apply right to the output of left".
func (p *parser) parsePostfix() (Node, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		// `.foo` continuation: a dot directly followed by an identifier.
		case p.cur().typ == tDot && p.peek().typ == tIdent:
			p.advance() // dot
			name := p.advance().val
			node = Pipe{Left: node, Right: Field{Name: name}}
		// `.[...]` continuation: dot then a bracket suffix.
		case p.cur().typ == tDot && p.peek().typ == tLBracket:
			p.advance() // dot
			suffix, err := p.parseBracketSuffix()
			if err != nil {
				return nil, err
			}
			node = Pipe{Left: node, Right: suffix}
		// `[...]` directly attached (e.g. `keys[0]`).
		case p.cur().typ == tLBracket:
			suffix, err := p.parseBracketSuffix()
			if err != nil {
				return nil, err
			}
			node = Pipe{Left: node, Right: suffix}
		// trailing `?` — error suppression.
		case p.cur().typ == tQuestion:
			p.advance()
			node = Try{Expr: node}
		default:
			return node, nil
		}
	}
}

// parseBracketSuffix parses a `[ ... ]` that follows a value. Empty brackets
// `[]` mean iterate; `[expr]` means index. The leading `[` is the current token.
func (p *parser) parseBracketSuffix() (Node, error) {
	if _, err := p.expect(tLBracket, "'['"); err != nil {
		return nil, err
	}
	if p.cur().typ == tRBracket {
		p.advance()
		return Iterate{}, nil
	}
	idx, err := p.parsePipe()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tRBracket, "']'"); err != nil {
		return nil, err
	}
	return Index{Index: idx}, nil
}

func (p *parser) parsePrimary() (Node, error) {
	t := p.cur()
	switch t.typ {
	case tDot:
		p.advance()
		// `.name` is a field; a bare `.` (or `.` before `[`) is identity, and
		// the bracket/dot is handled by parsePostfix.
		if p.cur().typ == tIdent {
			name := p.advance().val
			return Field{Name: name}, nil
		}
		return Identity{}, nil

	case tLBracket:
		// Array construction: `[ pipe? ]`.
		p.advance()
		if p.cur().typ == tRBracket {
			p.advance()
			return ArrayConstruct{Expr: nil}, nil
		}
		inner, err := p.parsePipe()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRBracket, "']'"); err != nil {
			return nil, err
		}
		return ArrayConstruct{Expr: inner}, nil

	case tLParen:
		p.advance()
		inner, err := p.parsePipe()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRParen, "')'"); err != nil {
			return nil, err
		}
		return inner, nil

	case tNumber:
		p.advance()
		f, err := parseFloat(t.val)
		if err != nil {
			return nil, err
		}
		return Literal{Value: f}, nil

	case tString:
		p.advance()
		return Literal{Value: t.val}, nil

	case tIdent:
		return p.parseIdent()

	default:
		return nil, fmt.Errorf("unexpected token %q at position %d", t.val, t.pos)
	}
}

// parseIdent handles bare identifiers: the literal keywords true/false/null, and
// builtin/function calls with optional parenthesised arguments.
func (p *parser) parseIdent() (Node, error) {
	name := p.advance().val
	switch name {
	case "true":
		return Literal{Value: true}, nil
	case "false":
		return Literal{Value: false}, nil
	case "null":
		return Literal{Value: nil}, nil
	}

	call := Call{Name: name}
	if p.cur().typ == tLParen {
		p.advance()
		for {
			arg, err := p.parsePipe()
			if err != nil {
				return nil, err
			}
			call.Args = append(call.Args, arg)
			if p.cur().typ == tComma {
				p.advance()
				continue
			}
			break
		}
		if _, err := p.expect(tRParen, "')'"); err != nil {
			return nil, err
		}
	}
	return call, nil
}
