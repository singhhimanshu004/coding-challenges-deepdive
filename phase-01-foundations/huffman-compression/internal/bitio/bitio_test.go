package bitio

import (
	"bytes"
	"io"
	"testing"
)

func TestWriteReadRoundTripBits(t *testing.T) {
	// A non-byte-aligned bit pattern: 13 bits.
	bits := []uint{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 1, 0, 1}

	var buf bytes.Buffer
	bw := NewWriter(&buf)
	for _, b := range bits {
		if err := bw.WriteBit(b); err != nil {
			t.Fatalf("WriteBit: %v", err)
		}
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// 13 bits -> 2 bytes (final byte zero-padded).
	if got := buf.Len(); got != 2 {
		t.Fatalf("expected 2 bytes, got %d", got)
	}

	br := NewReader(&buf)
	for i, want := range bits {
		got, err := br.ReadBit()
		if err != nil {
			t.Fatalf("ReadBit[%d]: %v", i, err)
		}
		if got != want {
			t.Fatalf("bit %d: got %d want %d", i, got, want)
		}
	}
}

func TestWriteBitsString(t *testing.T) {
	var buf bytes.Buffer
	bw := NewWriter(&buf)
	if err := bw.WriteBits("10110010"); err != nil {
		t.Fatal(err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 1 {
		t.Fatalf("expected 1 byte, got %d", buf.Len())
	}
	// MSB-first: "10110010" == 0xB2.
	if got := buf.Bytes()[0]; got != 0xB2 {
		t.Fatalf("got %#x want 0xB2", got)
	}
}

func TestReadBitEOF(t *testing.T) {
	br := NewReader(bytes.NewReader(nil))
	if _, err := br.ReadBit(); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestEmptyFlush(t *testing.T) {
	var buf bytes.Buffer
	bw := NewWriter(&buf)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected 0 bytes for empty flush, got %d", buf.Len())
	}
}
