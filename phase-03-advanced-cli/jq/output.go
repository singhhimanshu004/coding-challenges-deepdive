package main

// output.go — turn our JSON value model back into text. This is the inverse of
// the parser: a recursive *encoder*. It supports jq's two layouts (pretty with
// 2-space indent, and `-c` compact) plus optional ANSI colour and `-r` raw
// string output.

import (
	"strconv"
	"strings"
)

// encodeOptions bundles the user's output preferences so we don't thread four
// bool parameters through every recursive call.
type encodeOptions struct {
	compact  bool // -c : single line, no extra spaces
	color    bool // -C : ANSI colours
	sortKeys bool // -S : emit object keys in sorted order
}

// ANSI colour codes roughly matching jq's default palette.
const (
	colReset  = "\x1b[0m"
	colNull   = "\x1b[1;30m" // bright black
	colFalse  = "\x1b[0m"
	colTrue   = "\x1b[0m"
	colNumber = "\x1b[0m"
	colString = "\x1b[0;32m" // green
	colKey    = "\x1b[34;1m" // bold blue
)

// encodeValue renders a single top-level value as a string (without a trailing
// newline — the caller adds that).
func encodeValue(v any, opts encodeOptions) string {
	var sb strings.Builder
	encode(&sb, v, opts, 0)
	return sb.String()
}

func encode(sb *strings.Builder, v any, opts encodeOptions, depth int) {
	switch val := v.(type) {
	case nil:
		colorize(sb, colNull, "null", opts)
	case bool:
		if val {
			colorize(sb, colTrue, "true", opts)
		} else {
			colorize(sb, colFalse, "false", opts)
		}
	case float64:
		colorize(sb, colNumber, formatNumber(val), opts)
	case string:
		colorize(sb, colString, encodeString(val), opts)
	case []any:
		encodeArray(sb, val, opts, depth)
	case *Object:
		encodeObject(sb, val, opts, depth)
	default:
		sb.WriteString("null")
	}
}

func encodeArray(sb *strings.Builder, arr []any, opts encodeOptions, depth int) {
	if len(arr) == 0 {
		sb.WriteString("[]")
		return
	}
	sb.WriteByte('[')
	for i, el := range arr {
		if i > 0 {
			sb.WriteByte(',')
		}
		newline(sb, opts, depth+1)
		encode(sb, el, opts, depth+1)
	}
	newline(sb, opts, depth)
	sb.WriteByte(']')
}

func encodeObject(sb *strings.Builder, obj *Object, opts encodeOptions, depth int) {
	if obj.Len() == 0 {
		sb.WriteString("{}")
		return
	}
	keys := obj.Keys()
	if opts.sortKeys {
		keys = append([]string{}, keys...)
		sortStrings(keys)
	}
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		newline(sb, opts, depth+1)
		colorize(sb, colKey, encodeString(k), opts)
		sb.WriteByte(':')
		if !opts.compact {
			sb.WriteByte(' ')
		}
		val, _ := obj.Get(k)
		encode(sb, val, opts, depth+1)
	}
	newline(sb, opts, depth)
	sb.WriteByte('}')
}

// newline writes a line break + indentation in pretty mode; nothing in compact
// mode (that is what makes `-c` a single line).
func newline(sb *strings.Builder, opts encodeOptions, depth int) {
	if opts.compact {
		return
	}
	sb.WriteByte('\n')
	for i := 0; i < depth; i++ {
		sb.WriteString("  ") // two spaces per level, like jq
	}
}

func colorize(sb *strings.Builder, code, text string, opts encodeOptions) {
	if opts.color {
		sb.WriteString(code)
		sb.WriteString(text)
		sb.WriteString(colReset)
	} else {
		sb.WriteString(text)
	}
}

// formatNumber prints integral floats without a trailing ".0" (so 30 not 30.0)
// and uses the shortest round-trippable form otherwise — matching jq.
func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// encodeString quotes and escapes a Go string as a JSON string literal.
func encodeString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\t':
			sb.WriteString("\\t")
		case '\r':
			sb.WriteString("\\r")
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// sortStrings is a tiny insertion sort kept local so output.go has no extra
// imports; key lists are short.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
