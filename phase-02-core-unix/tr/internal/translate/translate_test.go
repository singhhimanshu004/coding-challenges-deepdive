package translate

import (
	"strings"
	"testing"
)

// transform is a small test helper: build a Transformer from a Spec, run the
// given input through it, and return the output string. It fails the test on
// any construction or streaming error.
func transform(t *testing.T, spec Spec, input string) string {
	t.Helper()
	tr, err := New(spec)
	if err != nil {
		t.Fatalf("New(%+v) error: %v", spec, err)
	}
	var out strings.Builder
	if err := tr.Run(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	return out.String()
}

func TestTranslateBasic(t *testing.T) {
	got := transform(t, Spec{Set1: "abc", Set2: "xyz"}, "cabbage")
	if want := "zxyyxge"; got != want {
		t.Errorf("translate: got %q want %q", got, want)
	}
}

func TestTranslateLowerToUpperRange(t *testing.T) {
	got := transform(t, Spec{Set1: "a-z", Set2: "A-Z"}, "Hello, World!")
	if want := "HELLO, WORLD!"; got != want {
		t.Errorf("range translate: got %q want %q", got, want)
	}
}

func TestTranslateShortSet2Pads(t *testing.T) {
	// SET2 shorter than SET1: its last rune repeats to fill.
	got := transform(t, Spec{Set1: "abcd", Set2: "x"}, "dcba")
	if want := "xxxx"; got != want {
		t.Errorf("pad translate: got %q want %q", got, want)
	}
}

func TestDelete(t *testing.T) {
	got := transform(t, Spec{Set1: "[:digit:]", Delete: true}, "a1b2c3")
	if want := "abc"; got != want {
		t.Errorf("delete: got %q want %q", got, want)
	}
}

func TestSqueezeOnly(t *testing.T) {
	got := transform(t, Spec{Set1: "a-z", Squeeze: true}, "aaabbbccd")
	if want := "abcd"; got != want {
		t.Errorf("squeeze: got %q want %q", got, want)
	}
}

func TestSqueezeOnlyTargetedSet(t *testing.T) {
	// Only spaces are in the squeeze set, so repeated letters survive.
	got := transform(t, Spec{Set1: " ", Squeeze: true}, "a   b    cc")
	if want := "a b cc"; got != want {
		t.Errorf("targeted squeeze: got %q want %q", got, want)
	}
}

func TestTranslateThenSqueeze(t *testing.T) {
	// Translate vowels to '_', then squeeze the resulting '_' runs.
	got := transform(t, Spec{Set1: "aeiou", Set2: "_", Squeeze: true}, "queueing")
	if want := "q_ng"; got != want {
		t.Errorf("translate+squeeze: got %q want %q", got, want)
	}
}

func TestComplementDelete(t *testing.T) {
	// Delete everything that is NOT a digit.
	got := transform(t, Spec{Set1: "[:digit:]", Delete: true, Complement: true}, "a1b2c3")
	if want := "123"; got != want {
		t.Errorf("complement delete: got %q want %q", got, want)
	}
}

func TestComplementTranslate(t *testing.T) {
	// Map every non-digit to '*'.
	got := transform(t, Spec{Set1: "0-9", Set2: "*", Complement: true}, "a1b2")
	if want := "*1*2"; got != want {
		t.Errorf("complement translate: got %q want %q", got, want)
	}
}

func TestClassUpperToLower(t *testing.T) {
	got := transform(t, Spec{Set1: "[:upper:]", Set2: "[:lower:]"}, "MixedCASE")
	if want := "mixedcase"; got != want {
		t.Errorf("class translate: got %q want %q", got, want)
	}
}

func TestMultibyteRunes(t *testing.T) {
	// Greek λ and accented é must each be treated as one rune, not bytes.
	got := transform(t, Spec{Set1: "λé", Set2: "LE"}, "λ-é-λ")
	if want := "L-E-L"; got != want {
		t.Errorf("multibyte translate: got %q want %q", got, want)
	}
}

func TestMultibyteDelete(t *testing.T) {
	got := transform(t, Spec{Set1: "λ", Delete: true}, "aλbλc")
	if want := "abc"; got != want {
		t.Errorf("multibyte delete: got %q want %q", got, want)
	}
}

func TestEmptyInput(t *testing.T) {
	got := transform(t, Spec{Set1: "a-z", Set2: "A-Z"}, "")
	if want := ""; got != want {
		t.Errorf("empty input: got %q want %q", got, want)
	}
}

func TestExpandSetRange(t *testing.T) {
	got, err := ExpandSet("a-e")
	if err != nil {
		t.Fatalf("ExpandSet error: %v", err)
	}
	if want := "abcde"; string(got) != want {
		t.Errorf("ExpandSet range: got %q want %q", string(got), want)
	}
}

func TestExpandSetEscapes(t *testing.T) {
	got, err := ExpandSet(`\t\n\\`)
	if err != nil {
		t.Fatalf("ExpandSet error: %v", err)
	}
	if want := "\t\n\\"; string(got) != want {
		t.Errorf("ExpandSet escapes: got %q want %q", string(got), want)
	}
}

func TestExpandSetClass(t *testing.T) {
	got, err := ExpandSet("[:digit:]")
	if err != nil {
		t.Fatalf("ExpandSet error: %v", err)
	}
	if want := "0123456789"; string(got) != want {
		t.Errorf("ExpandSet class: got %q want %q", string(got), want)
	}
}

func TestExpandSetUnknownClass(t *testing.T) {
	if _, err := ExpandSet("[:bogus:]"); err == nil {
		t.Errorf("ExpandSet: expected error for unknown class")
	}
}

func TestNewRejectsMissingSet2(t *testing.T) {
	if _, err := New(Spec{Set1: "abc"}); err == nil {
		t.Errorf("New: expected error when translating without SET2")
	}
}
