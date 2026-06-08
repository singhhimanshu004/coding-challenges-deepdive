package huffman

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, name string, data []byte) {
	t.Helper()
	enc, err := Compress(data)
	if err != nil {
		t.Fatalf("%s: Compress: %v", name, err)
	}
	dec, err := Decompress(enc)
	if err != nil {
		t.Fatalf("%s: Decompress: %v", name, err)
	}
	if !bytes.Equal(dec, data) {
		t.Fatalf("%s: round-trip mismatch: got %d bytes, want %d bytes", name, len(dec), len(data))
	}
}

func TestRoundTripVariedInputs(t *testing.T) {
	cases := map[string][]byte{
		"empty":             {},
		"single-byte":       {'A'},
		"single-symbol-run": bytes.Repeat([]byte{'a'}, 1000),
		"text":              []byte("the quick brown fox jumps over the lazy dog"),
		"repeated-text":     bytes.Repeat([]byte("abracadabra "), 100),
		"all-256-bytes":     allBytes(),
		"binary":            randomBytes(4096, 1),
		"newlines":          []byte("\n\n\nline1\nline2\n\n"),
	}
	for name, data := range cases {
		roundTrip(t, name, data)
	}
}

func TestCompressionActuallyShrinksSkewedData(t *testing.T) {
	// Highly skewed distribution should compress well below the original size.
	data := []byte(strings.Repeat("a", 9000) + strings.Repeat("b", 1000))
	enc, err := Compress(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(enc) >= len(data) {
		t.Fatalf("expected compression, got %d >= %d", len(enc), len(data))
	}
	roundTrip(t, "skewed", data)
}

func TestDecompressRejectsBadMagic(t *testing.T) {
	if _, err := Decompress([]byte("NOPE....")); err != ErrBadFormat {
		t.Fatalf("expected ErrBadFormat, got %v", err)
	}
	if _, err := Decompress(nil); err != ErrBadFormat {
		t.Fatalf("expected ErrBadFormat for empty, got %v", err)
	}
}

func TestCountFrequencies(t *testing.T) {
	f := CountFrequencies([]byte("aaab"))
	if f['a'] != 3 || f['b'] != 1 {
		t.Fatalf("unexpected freqs: %+v", f)
	}
}

func TestBuildCodesArePrefixFree(t *testing.T) {
	freqs := CountFrequencies([]byte("abracadabra"))
	codes := buildCodes(buildTree(freqs))
	for s1, c1 := range codes {
		for s2, c2 := range codes {
			if s1 == s2 {
				continue
			}
			if strings.HasPrefix(c2, c1) {
				t.Fatalf("code %q (%q) is a prefix of %q (%q)", c1, string(s1), c2, string(s2))
			}
		}
	}
}

func TestRatio(t *testing.T) {
	if got := Ratio(0, 10); got != 0 {
		t.Fatalf("ratio of empty original should be 0, got %v", got)
	}
	if got := Ratio(100, 25); got != 0.25 {
		t.Fatalf("got %v want 0.25", got)
	}
}

func TestRandomFuzzRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		n := rng.Intn(2000)
		data := make([]byte, n)
		for j := range data {
			// Bias toward a small alphabet so trees vary in shape.
			data[j] = byte(rng.Intn(rng.Intn(255) + 1))
		}
		roundTrip(t, "fuzz", data)
	}
}

func allBytes() []byte {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func randomBytes(n int, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	rng.Read(b)
	return b
}
