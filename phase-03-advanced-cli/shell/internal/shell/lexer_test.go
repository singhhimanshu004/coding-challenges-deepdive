package shell

import (
	"reflect"
	"testing"
)

// ---- TOKENIZER tests: quotes and escapes ---------------------------------

// words runs the tokenizer and returns just the WORD tokens as raw strings,
// asserting there were no operator tokens. Convenient for word-splitting checks.
func lexWords(t *testing.T, in string) []string {
	t.Helper()
	toks, err := tokenize(in)
	if err != nil {
		t.Fatalf("tokenize(%q) error: %v", in, err)
	}
	var out []string
	for _, tk := range toks {
		if tk.kind != tWord {
			t.Fatalf("tokenize(%q): unexpected operator token %v", in, tk.kind)
		}
		out = append(out, tk.word.raw())
	}
	return out
}

func TestTokenizeWordSplitting(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"echo hello world", []string{"echo", "hello", "world"}},
		{"  spaced   out  ", []string{"spaced", "out"}},
		{"a\tb", []string{"a", "b"}},
	}
	for _, c := range cases {
		if got := lexWords(t, c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("tokenize(%q) words = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTokenizeQuotes(t *testing.T) {
	// Double quotes keep spaces together.
	if got := lexWords(t, `echo "hello world"`); !reflect.DeepEqual(got, []string{"echo", "hello world"}) {
		t.Errorf("double quotes: got %v", got)
	}
	// Single quotes keep spaces together too.
	if got := lexWords(t, `echo 'hello world'`); !reflect.DeepEqual(got, []string{"echo", "hello world"}) {
		t.Errorf("single quotes: got %v", got)
	}
	// Adjacent quoted+unquoted pieces fuse into one word.
	if got := lexWords(t, `a"b c"d`); !reflect.DeepEqual(got, []string{"ab cd"}) {
		t.Errorf("adjacent quoting: got %v want %v", got, []string{"ab cd"})
	}
}

func TestTokenizeEscapes(t *testing.T) {
	if got := lexWords(t, `a\ b`); !reflect.DeepEqual(got, []string{"a b"}) {
		t.Errorf("backslash space: got %v want [a b]", got)
	}
	if got := lexWords(t, `echo \$HOME`); !reflect.DeepEqual(got, []string{"echo", "$HOME"}) {
		t.Errorf("escaped dollar: got %v", got)
	}
	// Empty quoted string is still a (single, empty) argument.
	if got := lexWords(t, `echo ""`); !reflect.DeepEqual(got, []string{"echo", ""}) {
		t.Errorf("empty quotes: got %v want [echo \"\"]", got)
	}
}

func TestTokenizeUnterminatedQuote(t *testing.T) {
	if _, err := tokenize(`echo "oops`); err == nil {
		t.Error("expected error for unterminated double quote")
	}
	if _, err := tokenize(`echo 'oops`); err == nil {
		t.Error("expected error for unterminated single quote")
	}
}

func TestTokenizeOperators(t *testing.T) {
	toks, err := tokenize(`a | b > c 2> d >> e < f ; g && h || i`)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []tokKind
	for _, tk := range toks {
		if tk.kind != tWord {
			kinds = append(kinds, tk.kind)
		}
	}
	want := []tokKind{tPipe, tGreat, tErrGreat, tDGreat, tLess, tSemi, tAndAnd, tOrOr}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("operator kinds = %v, want %v", kinds, want)
	}
}

func TestTokenizeStderrRedirectGlued(t *testing.T) {
	// `2>file` with no space must produce a stderr-redirect operator + filename,
	// not the word "2" followed by ">".
	toks, err := tokenize(`cmd 2>err.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 3 || toks[1].kind != tErrGreat || toks[2].word.raw() != "err.txt" {
		t.Fatalf("glued 2> not parsed correctly: %+v", toks)
	}
}
