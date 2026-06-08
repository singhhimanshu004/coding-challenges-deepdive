// Package translate implements the core of the `tr` filter: expanding the
// SET operands into concrete runes, and translating / deleting / squeezing /
// complementing a stream of runes.
//
// Go-for-Python note: a "package" in Go is a directory of .go files that share
// the same `package` name (here `translate`). It's the rough equivalent of a
// Python module/package. Anything with a Capitalised name (ExpandSet, Spec)
// is *exported* — visible to code that imports this package, like Python's
// "public" names. Lower-case names (decodeEscape) are private to the package,
// like a leading-underscore convention in Python, except Go actually enforces
// it at compile time.
package translate

import "fmt"

// charClasses maps a POSIX character-class name (the `alpha` in `[:alpha:]`)
// to the set of runes it stands for. tr only supports a handful of these; we
// implement the ones the challenge asks for plus a couple of obvious extras.
//
// Go-for-Python note: this is a package-level `var` initialised with a map
// literal. `map[string][]rune` is "map from string keys to slices of rune" —
// like a Python dict[str, list[str]] where each rune is a Unicode code point.
var charClasses = map[string][]rune{
	"alpha": rangeRunes('a', 'z', 'A', 'Z'),
	"digit": rangeRunes('0', '9'),
	"upper": rangeRunes('A', 'Z'),
	"lower": rangeRunes('a', 'z'),
	"alnum": rangeRunes('a', 'z', 'A', 'Z', '0', '9'),
	// space = the standard whitespace runes tr recognises.
	"space": {'\t', '\n', '\v', '\f', '\r', ' '},
	"blank": {'\t', ' '},
}

// rangeRunes is a tiny helper that expands one or more inclusive [lo, hi]
// pairs into a flat slice of runes. It expects an even number of arguments.
//
// Go-for-Python note: `args ...rune` is a variadic parameter, exactly like
// Python's `*args`. Inside the function `args` is a slice ([]rune).
func rangeRunes(args ...rune) []rune {
	var out []rune
	for i := 0; i+1 < len(args); i += 2 {
		for c := args[i]; c <= args[i+1]; c++ {
			out = append(out, c)
		}
	}
	return out
}

// ExpandSet turns a tr SET operand string into the explicit slice of runes it
// represents, handling three kinds of shorthand:
//
//	a-z          → a range of consecutive runes
//	[:alpha:]    → a POSIX character class
//	\n \t \\ …   → C-style backslash escapes
//
// Everything else is taken literally, one rune at a time. Order is preserved,
// which matters: for translation tr maps SET1[i] → SET2[i] positionally.
func ExpandSet(spec string) ([]rune, error) {
	// []rune(spec) decodes the UTF-8 string into a slice of Unicode code
	// points. This is the key to correct multibyte handling: we never index
	// raw bytes, so "é" or "λ" is a single element, not 2–4 bytes.
	in := []rune(spec)
	var out []rune

	i := 0
	for i < len(in) {
		// 1) POSIX character class: [:name:]
		if class, adv, ok := matchClass(in, i); ok {
			runes, found := charClasses[class]
			if !found {
				return nil, fmt.Errorf("unknown character class %q", "[:"+class+":]")
			}
			out = append(out, runes...)
			i += adv
			continue
		}

		// 2) Decode the (possibly escaped) rune at position i.
		lo, adv := decodeEscape(in, i)

		// 3) Is this the start of a range "lo-hi"? A '-' only forms a range
		//    when it sits *between* two characters, so we need a char after it.
		if i+adv < len(in) && in[i+adv] == '-' && i+adv+1 < len(in) {
			hi, adv2 := decodeEscape(in, i+adv+1)
			if hi < lo {
				return nil, fmt.Errorf("invalid range: %q-%q is descending", lo, hi)
			}
			for c := lo; c <= hi; c++ {
				out = append(out, c)
			}
			i += adv + 1 + adv2
			continue
		}

		// 4) Plain literal rune.
		out = append(out, lo)
		i += adv
	}
	return out, nil
}

// matchClass checks whether position i begins a `[:name:]` token. On success
// it returns the class name, how many runes to advance, and true.
func matchClass(in []rune, i int) (name string, advance int, ok bool) {
	if i+1 >= len(in) || in[i] != '[' || in[i+1] != ':' {
		return "", 0, false
	}
	// Scan forward for the closing ":]".
	for j := i + 2; j+1 < len(in); j++ {
		if in[j] == ':' && in[j+1] == ']' {
			return string(in[i+2 : j]), (j + 2) - i, true
		}
	}
	return "", 0, false
}

// decodeEscape reads one logical character starting at index i. If it sees a
// backslash escape it decodes it (returning the real rune and an advance of 2);
// otherwise it returns the literal rune and an advance of 1.
//
// Go-for-Python note: Go functions can return multiple values natively, like
// Python tuple-unpacking `lo, adv = decode(...)` but without building a tuple.
func decodeEscape(in []rune, i int) (r rune, advance int) {
	if in[i] != '\\' || i+1 >= len(in) {
		return in[i], 1
	}
	switch in[i+1] {
	case 'n':
		return '\n', 2
	case 't':
		return '\t', 2
	case 'r':
		return '\r', 2
	case 'f':
		return '\f', 2
	case 'v':
		return '\v', 2
	case 'a':
		return '\a', 2
	case 'b':
		return '\b', 2
	case '\\':
		return '\\', 2
	default:
		// Unknown escape: tr treats "\x" as a literal "x".
		return in[i+1], 2
	}
}
