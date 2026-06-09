package main

// jsonval.go — the JSON *value model* plus a hand-rolled, recursive-descent
// JSON parser. This is the Phase-1 "build your own JSON parser" mindset reused
// in Go: read characters left-to-right, and let the grammar's shape drive a set
// of mutually-recursive functions (parseValue → parseObject/parseArray → …).
//
// 🐍 Python analogy: in Python you'd reach for `json.loads`, which hands you
// `dict`/`list`/`str`/`int`/`float`/`bool`/`None`. We deliberately build the
// parser by hand for the learning value, and we model the same seven JSON types
// using Go's `any` (alias for `interface{}`):
//
//   JSON        Go representation
//   ----------  --------------------------------
//   null        nil
//   true/false  bool
//   number      float64
//   string      string
//   array       []any
//   object      *Object   (insertion-ordered — see below)
//
// We use *Object instead of Go's built-in map[string]any for one reason:
// real `jq` preserves the key order from the input document, but Go maps have a
// deliberately *randomised* iteration order. An ordered object lets our output
// match real jq byte-for-byte.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Object is a JSON object that remembers the order its keys first appeared in.
// `keys` is the source-order list; `m` is the lookup table. They are kept in
// sync by set().
//
// 🐍 Python analogy: this is essentially a `collections.OrderedDict` — except
// modern Python dicts are ordered anyway, whereas in Go we have to build the
// ordering ourselves.
type Object struct {
	keys []string
	m    map[string]any
}

// NewObject returns an empty ordered object ready to be filled.
func NewObject() *Object {
	return &Object{m: make(map[string]any)}
}

// Set inserts or updates a key. A brand-new key is appended to the order list;
// updating an existing key leaves its position untouched (same as jq / Python).
func (o *Object) Set(key string, val any) {
	if _, exists := o.m[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.m[key] = val
}

// Get returns the value for key and whether it was present (the comma-ok idiom).
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.m[key]
	return v, ok
}

// Keys returns the keys in insertion order. Callers that need them sorted (the
// `keys` builtin) sort a copy themselves.
func (o *Object) Keys() []string { return o.keys }

// Len reports how many keys the object has.
func (o *Object) Len() int { return len(o.keys) }

// --- The parser ------------------------------------------------------------

// jsonParser walks the input string with a single moving cursor `pos`. Keeping
// all the state in one struct (instead of threading an index through every
// function) is the idiomatic Go way to write a recursive-descent parser.
type jsonParser struct {
	s   string
	pos int
}

// ParseJSONStream parses the input as a *stream* of whitespace-separated JSON
// values (this is what jq consumes: `echo '1 2 3' | jq .` sees three inputs).
// It returns every top-level value it finds, in order.
func ParseJSONStream(input string) ([]any, error) {
	p := &jsonParser{s: input}
	var out []any
	for {
		p.skipWhitespace()
		if p.pos >= len(p.s) {
			break // clean EOF — no more values
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// skipWhitespace advances past the JSON insignificant whitespace set.
func (p *jsonParser) skipWhitespace() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// parseValue is the heart of recursive descent: peek at the next significant
// byte and dispatch to the function that knows how to parse that shape.
func (p *jsonParser) parseValue() (any, error) {
	p.skipWhitespace()
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("unexpected end of JSON input")
	}
	c := p.s[p.pos]
	switch {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"':
		return p.parseString()
	case c == 't', c == 'f':
		return p.parseBool()
	case c == 'n':
		return p.parseNull()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	default:
		return nil, fmt.Errorf("unexpected character %q at position %d", c, p.pos)
	}
}

func (p *jsonParser) parseObject() (any, error) {
	obj := NewObject()
	p.pos++ // consume '{'
	p.skipWhitespace()
	if p.pos < len(p.s) && p.s[p.pos] == '}' {
		p.pos++ // empty object
		return obj, nil
	}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.s) || p.s[p.pos] != '"' {
			return nil, fmt.Errorf("expected string key in object at position %d", p.pos)
		}
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.pos >= len(p.s) || p.s[p.pos] != ':' {
			return nil, fmt.Errorf("expected ':' after object key at position %d", p.pos)
		}
		p.pos++ // consume ':'
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.Set(key.(string), val)
		p.skipWhitespace()
		if p.pos >= len(p.s) {
			return nil, fmt.Errorf("unterminated object")
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
			continue
		case '}':
			p.pos++
			return obj, nil
		default:
			return nil, fmt.Errorf("expected ',' or '}' in object at position %d", p.pos)
		}
	}
}

func (p *jsonParser) parseArray() (any, error) {
	arr := []any{}
	p.pos++ // consume '['
	p.skipWhitespace()
	if p.pos < len(p.s) && p.s[p.pos] == ']' {
		p.pos++ // empty array
		return arr, nil
	}
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
		p.skipWhitespace()
		if p.pos >= len(p.s) {
			return nil, fmt.Errorf("unterminated array")
		}
		switch p.s[p.pos] {
		case ',':
			p.pos++
			continue
		case ']':
			p.pos++
			return arr, nil
		default:
			return nil, fmt.Errorf("expected ',' or ']' in array at position %d", p.pos)
		}
	}
}

// parseString returns the decoded Go string (handling escapes incl. \uXXXX and
// UTF-16 surrogate pairs). It returns `any` so it can be used directly both as a
// value and as an object key.
func (p *jsonParser) parseString() (any, error) {
	p.pos++ // consume opening quote
	var sb strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch c {
		case '"':
			p.pos++ // consume closing quote
			return sb.String(), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.s) {
				return nil, fmt.Errorf("unterminated escape in string")
			}
			esc := p.s[p.pos]
			switch esc {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				r, err := p.parseUnicodeEscape()
				if err != nil {
					return nil, err
				}
				sb.WriteRune(r)
				continue // parseUnicodeEscape already advanced pos
			default:
				return nil, fmt.Errorf("invalid escape \\%c", esc)
			}
			p.pos++
		default:
			sb.WriteByte(c)
			p.pos++
		}
	}
	return nil, fmt.Errorf("unterminated string")
}

// parseUnicodeEscape decodes a \uXXXX escape (already past the 'u'), combining a
// high+low surrogate pair into a single rune when present.
func (p *jsonParser) parseUnicodeEscape() (rune, error) {
	hi, err := p.readHex4()
	if err != nil {
		return 0, err
	}
	if utf16.IsSurrogate(rune(hi)) {
		// Expect a following \uXXXX low surrogate.
		if p.pos+1 < len(p.s) && p.s[p.pos] == '\\' && p.s[p.pos+1] == 'u' {
			p.pos += 2
			lo, err := p.readHex4()
			if err != nil {
				return 0, err
			}
			return utf16.DecodeRune(rune(hi), rune(lo)), nil
		}
		return '\uFFFD', nil // lone surrogate → replacement char
	}
	return rune(hi), nil
}

// readHex4 reads exactly four hex digits starting at pos (which sits on the 'u'),
// returning their numeric value and leaving pos just past them.
func (p *jsonParser) readHex4() (int, error) {
	p.pos++ // consume 'u'
	if p.pos+4 > len(p.s) {
		return 0, fmt.Errorf("incomplete \\u escape")
	}
	n, err := strconv.ParseInt(p.s[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid \\u escape: %v", err)
	}
	p.pos += 4
	return int(n), nil
}

func (p *jsonParser) parseBool() (any, error) {
	if strings.HasPrefix(p.s[p.pos:], "true") {
		p.pos += 4
		return true, nil
	}
	if strings.HasPrefix(p.s[p.pos:], "false") {
		p.pos += 5
		return false, nil
	}
	return nil, fmt.Errorf("invalid literal at position %d", p.pos)
}

func (p *jsonParser) parseNull() (any, error) {
	if strings.HasPrefix(p.s[p.pos:], "null") {
		p.pos += 4
		return nil, nil
	}
	return nil, fmt.Errorf("invalid literal at position %d", p.pos)
}

// parseNumber scans a JSON number (int or float, with optional exponent) and
// returns it as a float64 — the single numeric type jq works with internally.
func (p *jsonParser) parseNumber() (any, error) {
	start := p.pos
	if p.pos < len(p.s) && p.s[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			p.pos++
		} else {
			break
		}
	}
	f, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number %q", p.s[start:p.pos])
	}
	return f, nil
}
