package main

// lexer.go — the *filter language* lexer (a.k.a. tokenizer / scanner).
//
// This is step one of the classic three-stage pipeline you also used for the
// JSON parser: lex → parse → evaluate. The lexer turns the raw filter text the
// user typed (e.g. `.[] | select(.age > 30)`) into a flat slice of tokens, so
// the parser never has to think about whitespace or how many characters make up
// `>=`.
//
// 🐍 Python analogy: this is like Python's `tokenize` module — it chops source
// text into typed chunks (NAME, OP, NUMBER…) before any grammar rules apply.

import (
	"fmt"
	"strings"
)

// tokenType enumerates every kind of token in our jq subset.
//
// 🐍 Python analogy: Go has no `enum` keyword. The idiom is a named integer
// type plus a block of `iota` constants — iota auto-increments 0,1,2,… down the
// const block, giving each name a distinct value.
type tokenType int

const (
	tEOF      tokenType = iota
	tDot                // .
	tIdent              // foo, length, select, true, false, null
	tNumber             // 42, 3.14
	tString             // "hello"
	tLBracket           // [
	tRBracket           // ]
	tLParen             // (
	tRParen             // )
	tPipe               // |
	tComma              // ,
	tQuestion           // ?
	tEq                 // ==
	tNe                 // !=
	tLt                 // <
	tGt                 // >
	tLe                 // <=
	tGe                 // >=
	tPlus               // +
	tMinus              // -
	tStar               // *
	tSlash              // /
)

// token is one lexical unit: its kind, its literal text, and where it started
// (the position is used to produce helpful parse-error messages).
type token struct {
	typ tokenType
	val string
	pos int
}

// lex scans the whole filter string into a token slice terminated by tEOF.
// Returning the full slice up-front (rather than an iterator) keeps the parser
// simple: it can freely peek ahead and backtrack by index.
func lex(input string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(input) {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++ // skip whitespace
		case c == '.':
			toks = append(toks, token{tDot, ".", i})
			i++
		case c == '[':
			toks = append(toks, token{tLBracket, "[", i})
			i++
		case c == ']':
			toks = append(toks, token{tRBracket, "]", i})
			i++
		case c == '(':
			toks = append(toks, token{tLParen, "(", i})
			i++
		case c == ')':
			toks = append(toks, token{tRParen, ")", i})
			i++
		case c == '|':
			toks = append(toks, token{tPipe, "|", i})
			i++
		case c == ',':
			toks = append(toks, token{tComma, ",", i})
			i++
		case c == '?':
			toks = append(toks, token{tQuestion, "?", i})
			i++
		case c == '+':
			toks = append(toks, token{tPlus, "+", i})
			i++
		case c == '*':
			toks = append(toks, token{tStar, "*", i})
			i++
		case c == '/':
			toks = append(toks, token{tSlash, "/", i})
			i++
		case c == '-':
			// A '-' is the start of a negative number literal only when a digit
			// follows; otherwise it is the subtraction operator.
			if i+1 < len(input) && isDigit(input[i+1]) {
				tok, next := lexNumber(input, i)
				toks = append(toks, tok)
				i = next
			} else {
				toks = append(toks, token{tMinus, "-", i})
				i++
			}
		case c == '=':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tEq, "==", i})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '=' at position %d (did you mean '=='?)", i)
			}
		case c == '!':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tNe, "!=", i})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '!' at position %d", i)
			}
		case c == '<':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tLe, "<=", i})
				i += 2
			} else {
				toks = append(toks, token{tLt, "<", i})
				i++
			}
		case c == '>':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tGe, ">=", i})
				i += 2
			} else {
				toks = append(toks, token{tGt, ">", i})
				i++
			}
		case c == '"':
			tok, next, err := lexString(input, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok)
			i = next
		case isDigit(c):
			tok, next := lexNumber(input, i)
			toks = append(toks, tok)
			i = next
		case isIdentStart(c):
			tok, next := lexIdent(input, i)
			toks = append(toks, tok)
			i = next
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", c, i)
		}
	}
	toks = append(toks, token{tEOF, "", len(input)})
	return toks, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isIdentStart / isIdentChar define what may appear in a field name or builtin
// identifier. jq field names allow letters, digits and underscores.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func lexNumber(input string, start int) (token, int) {
	i := start
	if input[i] == '-' {
		i++
	}
	for i < len(input) && (isDigit(input[i]) || input[i] == '.') {
		i++
	}
	return token{tNumber, input[start:i], start}, i
}

func lexIdent(input string, start int) (token, int) {
	i := start
	for i < len(input) && isIdentChar(input[i]) {
		i++
	}
	return token{tIdent, input[start:i], start}, i
}

// lexString scans a double-quoted string literal, decoding the common escapes.
func lexString(input string, start int) (token, int, error) {
	var sb strings.Builder
	i := start + 1 // skip opening quote
	for i < len(input) {
		c := input[i]
		if c == '"' {
			return token{tString, sb.String(), start}, i + 1, nil
		}
		if c == '\\' {
			i++
			if i >= len(input) {
				break
			}
			switch input[i] {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte(input[i])
			}
			i++
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return token{}, 0, fmt.Errorf("unterminated string literal in filter")
}
