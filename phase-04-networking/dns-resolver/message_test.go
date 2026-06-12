package main

// Table-driven unit tests for the DNS wire format. They run entirely on crafted
// byte slices — NO network is touched — so they are fast and hermetic.

import (
	"bytes"
	"testing"
)

// TestHeaderRoundTrip packs a header and unpacks it back, asserting the exact
// 12 big-endian bytes along the way.
func TestHeaderRoundTrip(t *testing.T) {
	h := Header{ID: 0xABCD, Flags: flagRD, QDCount: 1, ANCount: 2, NSCount: 3, ARCount: 4}
	got := h.pack(nil)

	want := []byte{
		0xAB, 0xCD, // ID
		0x01, 0x00, // Flags: RD set (0x0100)
		0x00, 0x01, // QDCount
		0x00, 0x02, // ANCount
		0x00, 0x03, // NSCount
		0x00, 0x04, // ARCount
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("header pack mismatch:\n got=% x\nwant=% x", got, want)
	}

	back, err := unpackHeader(got)
	if err != nil {
		t.Fatalf("unpackHeader: %v", err)
	}
	if back != h {
		t.Errorf("round-trip header = %+v; want %+v", back, h)
	}
}

func TestUnpackHeaderTooShort(t *testing.T) {
	if _, err := unpackHeader([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for short header")
	}
}

// TestEncodeName checks label encoding for several names including the root.
func TestEncodeName(t *testing.T) {
	cases := []struct {
		name string
		want []byte
	}{
		{"example.com", []byte{7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}},
		{"a.bc", []byte{1, 'a', 2, 'b', 'c', 0}},
		{"", []byte{0}}, // root
		{"trailing.dot.", []byte{8, 't', 'r', 'a', 'i', 'l', 'i', 'n', 'g', 3, 'd', 'o', 't', 0}},
	}
	for _, c := range cases {
		got := encodeName(nil, c.name)
		if !bytes.Equal(got, c.want) {
			t.Errorf("encodeName(%q) = % x; want % x", c.name, got, c.want)
		}
	}
}

// TestDecodeNameSimple decodes a plain (uncompressed) name and checks the
// returned continuation offset.
func TestDecodeNameSimple(t *testing.T) {
	msg := []byte{7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0xFF}
	name, off, err := decodeName(msg, 0)
	if err != nil {
		t.Fatalf("decodeName: %v", err)
	}
	if name != "example.com" {
		t.Errorf("name = %q; want example.com", name)
	}
	if off != 13 { // index of the trailing 0xFF sentinel
		t.Errorf("offset = %d; want 13", off)
	}
}

// TestDecodeNameCompression is the headline test: a name that ends in a 0xC0
// POINTER back to an earlier name. This is the classic gotcha the README warns
// about.
func TestDecodeNameCompression(t *testing.T) {
	// Lay out a message where "example.com" lives at offset 12, and a second
	// name "www" + pointer-to-12 lives right after it.
	msg := make([]byte, 12) // pretend header occupies 0..11
	base := len(msg)
	msg = append(msg, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0) // "example.com" @ 12
	wwwAt := len(msg)
	msg = append(msg, 3, 'w', 'w', 'w') // "www" label
	// 0xC0 pointer to offset 12 (base): top two bits set, low 14 bits = 12.
	msg = append(msg, 0xC0, byte(base))
	msg = append(msg, 0xEE) // sentinel after the pointer

	// Decoding the compressed name must yield "www.example.com".
	name, off, err := decodeName(msg, wwwAt)
	if err != nil {
		t.Fatalf("decodeName(compressed): %v", err)
	}
	if name != "www.example.com" {
		t.Errorf("name = %q; want www.example.com", name)
	}
	// The continuation offset must point JUST PAST the 2-byte pointer in the
	// original stream — not wherever the jump led. That's the subtle part.
	// "www" label = 1 length byte + 3 chars = 4 bytes, then 2 pointer bytes.
	if off != wwwAt+6 {
		t.Errorf("offset = %d; want %d", off, wwwAt+6)
	}
}

// TestDecodeNamePointerLoop ensures a self-referential pointer is rejected
// rather than looping forever.
func TestDecodeNamePointerLoop(t *testing.T) {
	// A pointer at offset 0 that points to offset 0 — an infinite loop.
	msg := []byte{0xC0, 0x00}
	if _, _, err := decodeName(msg, 0); err == nil {
		t.Error("expected error for pointer loop, got nil")
	}
}

// TestUnpackRR_A crafts a complete A record (with a compressed owner name) and
// checks every decoded field.
func TestUnpackRR_A(t *testing.T) {
	msg := make([]byte, 12)
	base := len(msg)
	msg = append(msg, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0) // owner @ 12
	rrAt := len(msg)
	// NAME = pointer to 12
	msg = append(msg, 0xC0, byte(base))
	// TYPE=A(1), CLASS=IN(1), TTL=300, RDLENGTH=4, RDATA=93.184.216.34
	msg = append(msg,
		0x00, 0x01, // TYPE A
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x01, 0x2C, // TTL 300
		0x00, 0x04, // RDLENGTH 4
		93, 184, 216, 34, // the IP
	)

	rr, _, err := unpackRR(msg, rrAt)
	if err != nil {
		t.Fatalf("unpackRR: %v", err)
	}
	if rr.Name != "example.com" {
		t.Errorf("Name = %q; want example.com", rr.Name)
	}
	if rr.Type != TypeA {
		t.Errorf("Type = %d; want %d (A)", rr.Type, TypeA)
	}
	if rr.Class != ClassIN {
		t.Errorf("Class = %d; want %d (IN)", rr.Class, ClassIN)
	}
	if rr.TTL != 300 {
		t.Errorf("TTL = %d; want 300", rr.TTL)
	}
	if rr.Data != "93.184.216.34" {
		t.Errorf("Data = %q; want 93.184.216.34", rr.Data)
	}
}

// TestUnpackRR_MX checks MX RDATA decoding: a 2-byte preference followed by a
// (here compressed) mail-host name.
func TestUnpackRR_MX(t *testing.T) {
	msg := make([]byte, 12)
	base := len(msg)
	msg = append(msg, 4, 'm', 'a', 'i', 'l', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0)
	rrAt := len(msg)
	msg = append(msg, 0xC0, byte(base)) // owner NAME -> "mail.example.com"
	// TYPE=MX(15), CLASS=IN, TTL=60, RDLENGTH=3, RDATA= pref(10) + pointer->base
	msg = append(msg,
		0x00, 0x0F, // TYPE MX
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3C, // TTL 60
		0x00, 0x04, // RDLENGTH 4 (2 pref + 2 pointer)
		0x00, 0x0A, // preference 10
		0xC0, byte(base), // mail host -> "mail.example.com"
	)

	rr, _, err := unpackRR(msg, rrAt)
	if err != nil {
		t.Fatalf("unpackRR MX: %v", err)
	}
	if rr.Type != TypeMX {
		t.Fatalf("Type = %d; want MX", rr.Type)
	}
	if rr.Data != "10 mail.example.com" {
		t.Errorf("Data = %q; want '10 mail.example.com'", rr.Data)
	}
}

// TestBuildQueryAndParse builds a query, then parses it back as if it were a
// message — proving header+question encode and decode are consistent.
func TestBuildQueryAndParse(t *testing.T) {
	q := buildQuery(0x1234, "www.example.com", TypeAAAA, true)

	m, err := parseMessage(q)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if m.Header.ID != 0x1234 {
		t.Errorf("ID = %#x; want 0x1234", m.Header.ID)
	}
	if m.Header.Flags&flagRD == 0 {
		t.Error("RD flag should be set")
	}
	if len(m.Questions) != 1 {
		t.Fatalf("got %d questions; want 1", len(m.Questions))
	}
	got := m.Questions[0]
	if got.Name != "www.example.com" || got.Type != TypeAAAA || got.Class != ClassIN {
		t.Errorf("question = %+v; want {www.example.com AAAA IN}", got)
	}
}

// TestParseFullResponse decodes a realistic response containing a question and
// two answer records that BOTH use name compression — the most representative
// end-to-end decode.
func TestParseFullResponse(t *testing.T) {
	var b []byte
	// Header: id=0x0001, flags=0x8180 (QR+RD+RA), QD=1, AN=2, NS=0, AR=0.
	h := Header{ID: 1, Flags: flagQR | flagRD | flagRA, QDCount: 1, ANCount: 2}
	b = h.pack(b)
	qnameAt := len(b) // remember where the question name starts (offset 12)
	// Question: example.com A IN
	b = encodeName(b, "example.com")
	b = append(b, 0x00, 0x01, 0x00, 0x01)

	// Answer 1: owner = pointer to qnameAt, A, TTL 300, 1.2.3.4
	b = append(b, 0xC0, byte(qnameAt), 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x01, 0x2C, 0x00, 0x04, 1, 2, 3, 4)
	// Answer 2: owner = pointer to qnameAt, A, TTL 300, 5.6.7.8
	b = append(b, 0xC0, byte(qnameAt), 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x01, 0x2C, 0x00, 0x04, 5, 6, 7, 8)

	m, err := parseMessage(b)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if len(m.Answers) != 2 {
		t.Fatalf("got %d answers; want 2", len(m.Answers))
	}
	if m.Answers[0].Data != "1.2.3.4" || m.Answers[1].Data != "5.6.7.8" {
		t.Errorf("answers = %q, %q; want 1.2.3.4, 5.6.7.8", m.Answers[0].Data, m.Answers[1].Data)
	}
	if m.Answers[0].Name != "example.com" {
		t.Errorf("answer owner = %q; want example.com", m.Answers[0].Name)
	}
}

// TestParseType / TestTypeName round-trip the CLI type mnemonics.
func TestParseTypeRoundTrip(t *testing.T) {
	for _, name := range []string{"A", "AAAA", "NS", "CNAME", "MX"} {
		typ, err := parseType(name)
		if err != nil {
			t.Fatalf("parseType(%q): %v", name, err)
		}
		if typeName(typ) != name {
			t.Errorf("typeName(parseType(%q)) = %q", name, typeName(typ))
		}
	}
	if _, err := parseType("BOGUS"); err == nil {
		t.Error("expected error for unknown type")
	}
}
