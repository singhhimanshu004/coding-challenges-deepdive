package codec

import (
	"bytes"
	"fmt"
	"testing"

	"bloom/internal/bloom"
)

// TestRoundTrip proves Save→Load is lossless: a filter restored from disk must
// answer membership queries identically to the original, with the same m and k.
func TestRoundTrip(t *testing.T) {
	orig, err := bloom.New(1000, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	words := make([]string, 1000)
	for i := range words {
		words[i] = fmt.Sprintf("entry-%d", i)
		orig.AddString(words[i])
	}

	var buf bytes.Buffer
	if err := Save(&buf, orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.M() != orig.M() || loaded.K() != orig.K() {
		t.Fatalf("params changed: got m=%d k=%d, want m=%d k=%d",
			loaded.M(), loaded.K(), orig.M(), orig.K())
	}

	// Every inserted word still present after the round-trip.
	for _, w := range words {
		if !loaded.ContainsString(w) {
			t.Fatalf("loaded filter missing inserted word %q", w)
		}
	}
	// The bit arrays must be byte-for-byte identical.
	if !bytes.Equal(loaded.Bits().Bytes(), orig.Bits().Bytes()) {
		t.Fatal("bit arrays differ after round-trip")
	}
}

func TestLoadRejectsBadMagic(t *testing.T) {
	bad := []byte("XXXXnot a real filter file")
	if _, err := Load(bytes.NewReader(bad)); err != ErrBadFormat {
		t.Fatalf("expected ErrBadFormat, got %v", err)
	}
}

func TestLoadRejectsTruncated(t *testing.T) {
	orig, _ := bloom.New(100, 0.01)
	orig.AddString("hello")

	var buf bytes.Buffer
	if err := Save(&buf, orig); err != nil {
		t.Fatal(err)
	}

	// Lop off the tail so the declared payload length cannot be satisfied.
	full := buf.Bytes()
	truncated := full[:len(full)-3]
	if _, err := Load(bytes.NewReader(truncated)); err != ErrBadFormat {
		t.Fatalf("expected ErrBadFormat on truncated input, got %v", err)
	}
}

func TestLoadRejectsEmpty(t *testing.T) {
	if _, err := Load(bytes.NewReader(nil)); err != ErrBadFormat {
		t.Fatalf("expected ErrBadFormat on empty input, got %v", err)
	}
}

// TestSingleWordRoundTrip — the smallest filter must serialize and reload too.
func TestSingleWordRoundTrip(t *testing.T) {
	orig, err := bloom.New(1, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	orig.AddString("lonely")

	var buf bytes.Buffer
	if err := Save(&buf, orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ContainsString("lonely") {
		t.Fatal("single word lost in round-trip")
	}
}
