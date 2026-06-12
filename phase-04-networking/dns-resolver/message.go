package main

// This file is the heart of the challenge: it HAND-ENCODES and HAND-DECODES the
// DNS wire format described by RFC 1035. No helper library parses these bytes
// for us — we read and write every field ourselves with encoding/binary.
//
// 🐍➡️🐹 Python-dev orientation:
//   - A []byte is Go's mutable byte buffer. Think `bytearray`. A string is an
//     immutable view of bytes; converting is []byte(s) / string(b).
//   - encoding/binary is Go's `struct.pack`/`struct.unpack`. DNS is BIG-ENDIAN
//     ("network byte order"), so we always use binary.BigEndian.
//   - There are no classes; we use structs (plain data) with methods attached
//     via a receiver, e.g. func (h Header) pack(). That's just a function whose
//     first argument is the struct.

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Record / class type constants (the small subset we care about).
// ---------------------------------------------------------------------------

// DNS resource-record TYPE values (RFC 1035 §3.2.2, RFC 3596 for AAAA).
const (
	TypeA     uint16 = 1  // IPv4 address
	TypeNS    uint16 = 2  // authoritative name server
	TypeCNAME uint16 = 5  // canonical name (an alias)
	TypeMX    uint16 = 15 // mail exchange
	TypeAAAA  uint16 = 28 // IPv6 address
)

// ClassIN is the only DNS class anyone uses today: the Internet class.
const ClassIN uint16 = 1

// Header flag bit layout inside the 16-bit flags word (RFC 1035 §4.1.1):
//
//	 0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//	|QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
//	+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
const (
	flagQR = 1 << 15 // 0 = query, 1 = response
	flagRD = 1 << 8  // recursion desired (set by the client)
	flagRA = 1 << 7  // recursion available (set by the server)
	flagTC = 1 << 9  // truncated (answer too big for UDP; retry over TCP)
)

// ---------------------------------------------------------------------------
// Header — the fixed 12 bytes that begin every DNS message.
// ---------------------------------------------------------------------------

// Header mirrors the 12-byte DNS header. Every field is a 16-bit big-endian
// integer, so the struct is exactly 6 * uint16 = 12 bytes on the wire.
type Header struct {
	ID      uint16 // request id, echoed back in the response so we can match them
	Flags   uint16 // QR/Opcode/AA/TC/RD/RA/Z/RCODE packed together
	QDCount uint16 // number of entries in the question section
	ANCount uint16 // number of resource records in the answer section
	NSCount uint16 // number of name-server (authority) records
	ARCount uint16 // number of additional records
}

// pack appends the header as 12 big-endian bytes to buf and returns the result.
//
// 🐍 binary.BigEndian.PutUint16 is struct.pack(">H", n): write the two bytes
// most-significant-first. We append to a growing slice rather than seeking in a
// fixed buffer.
func (h Header) pack(buf []byte) []byte {
	var tmp [12]byte
	binary.BigEndian.PutUint16(tmp[0:], h.ID)
	binary.BigEndian.PutUint16(tmp[2:], h.Flags)
	binary.BigEndian.PutUint16(tmp[4:], h.QDCount)
	binary.BigEndian.PutUint16(tmp[6:], h.ANCount)
	binary.BigEndian.PutUint16(tmp[8:], h.NSCount)
	binary.BigEndian.PutUint16(tmp[10:], h.ARCount)
	return append(buf, tmp[:]...)
}

// unpackHeader reads the 12-byte header at the start of msg.
func unpackHeader(msg []byte) (Header, error) {
	if len(msg) < 12 {
		return Header{}, fmt.Errorf("message too short for header: %d bytes", len(msg))
	}
	return Header{
		ID:      binary.BigEndian.Uint16(msg[0:]),
		Flags:   binary.BigEndian.Uint16(msg[2:]),
		QDCount: binary.BigEndian.Uint16(msg[4:]),
		ANCount: binary.BigEndian.Uint16(msg[6:]),
		NSCount: binary.BigEndian.Uint16(msg[8:]),
		ARCount: binary.BigEndian.Uint16(msg[10:]),
	}, nil
}

// rcode pulls the low 4 bits of the flags word: the response code (0 = NOERROR,
// 3 = NXDOMAIN, ...).
func (h Header) rcode() int { return int(h.Flags & 0x0F) }

// ---------------------------------------------------------------------------
// Question — what we are asking about.
// ---------------------------------------------------------------------------

// Question is one entry in the question section: a name plus the type/class we
// want. A typical query has exactly one Question.
type Question struct {
	Name  string // e.g. "www.example.com" (dot-separated, no trailing dot)
	Type  uint16 // TypeA, TypeAAAA, ...
	Class uint16 // almost always ClassIN
}

// pack appends the question: the encoded QNAME, then QTYPE and QCLASS.
func (q Question) pack(buf []byte) []byte {
	buf = encodeName(buf, q.Name)
	var tc [4]byte
	binary.BigEndian.PutUint16(tc[0:], q.Type)
	binary.BigEndian.PutUint16(tc[2:], q.Class)
	return append(buf, tc[:]...)
}

// ---------------------------------------------------------------------------
// QNAME encoding — labels, not a plain string.
// ---------------------------------------------------------------------------

// encodeName writes a domain name in DNS label form and appends it to buf.
//
// A name is NOT stored as "www.example.com\0". Instead each dot-separated label
// is length-prefixed, and a zero-length label (a single 0x00 byte) terminates
// the name:
//
//	www.example.com  ->  3 'w' 'w' 'w' 7 'e' 'x' 'a' 'm' 'p' 'l' 'e' 3 'c' 'o' 'm' 0
//
// The root (an empty name "") encodes as just a single 0x00.
func encodeName(buf []byte, name string) []byte {
	name = strings.TrimSuffix(name, ".") // tolerate a trailing dot
	if name != "" {
		for _, label := range strings.Split(name, ".") {
			// A label is at most 63 bytes; the top two bits of the length byte
			// are reserved for the compression-pointer marker (0xC0).
			if len(label) > 63 {
				label = label[:63]
			}
			buf = append(buf, byte(len(label)))
			buf = append(buf, label...)
		}
	}
	return append(buf, 0x00) // the terminating zero-length label
}

// ---------------------------------------------------------------------------
// QNAME decoding — and the famous NAME COMPRESSION pointer.
// ---------------------------------------------------------------------------

// decodeName reads a (possibly compressed) domain name beginning at offset off
// in the FULL message msg. It returns the decoded dotted name and the offset of
// the first byte AFTER the name *in the original stream*.
//
// ── The classic gotcha: DNS NAME COMPRESSION ──────────────────────────────
// To save space, a name can end with a POINTER to a name that already appeared
// earlier in the same message. A length byte whose top two bits are set (i.e.
// byte & 0xC0 == 0xC0) is not a length at all: that byte and the next form a
// 14-bit OFFSET from the start of the message. We must jump there and keep
// reading labels — and that target may itself contain more pointers.
//
// Two things make this tricky and are the source of most resolver bugs:
//  1. The offset we RETURN to the caller is the position right after the 2-byte
//     pointer in the original stream — NOT wherever the jump led us. So we
//     record that "return" position the first time we follow a pointer.
//  2. A malicious or buggy packet could point in a loop. We cap the number of
//     jumps to guarantee termination.
func decodeName(msg []byte, off int) (string, int, error) {
	var labels []string
	pos := off
	returnOff := -1 // where the caller should continue; set on first pointer
	jumps := 0

	for {
		if pos >= len(msg) {
			return "", 0, fmt.Errorf("name runs past end of message at %d", pos)
		}
		b := msg[pos]

		switch {
		case b&0xC0 == 0xC0:
			// Compression pointer: this byte + the next = a 14-bit offset.
			if pos+1 >= len(msg) {
				return "", 0, fmt.Errorf("truncated compression pointer at %d", pos)
			}
			if returnOff == -1 {
				// The caller resumes just past these two pointer bytes.
				returnOff = pos + 2
			}
			ptr := int(binary.BigEndian.Uint16(msg[pos:]) & 0x3FFF)
			jumps++
			if jumps > 64 {
				return "", 0, fmt.Errorf("too many compression pointers (loop?)")
			}
			pos = ptr // jump and keep reading at the target

		case b == 0x00:
			// Zero-length label: the name ends here.
			pos++
			if returnOff == -1 {
				returnOff = pos
			}
			return strings.Join(labels, "."), returnOff, nil

		default:
			// A normal label: b bytes of text follow the length byte.
			length := int(b)
			start := pos + 1
			if start+length > len(msg) {
				return "", 0, fmt.Errorf("label runs past end of message at %d", pos)
			}
			labels = append(labels, string(msg[start:start+length]))
			pos = start + length
		}
	}
}

// ---------------------------------------------------------------------------
// Resource record — the answer/authority/additional entries.
// ---------------------------------------------------------------------------

// ResourceRecord is one parsed RR. We keep both the raw RDATA bytes and a
// human-readable Data string (decoded according to Type) so callers can use
// whichever they need.
type ResourceRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	RData []byte // raw RDATA, exactly RDLENGTH bytes
	Data  string // decoded form: an IP, a hostname, "pref host" for MX, ...
}

// unpackRR reads one resource record starting at offset off in msg and returns
// the record plus the offset just past it.
//
// RR layout (RFC 1035 §3.2.1):
//
//	NAME  (compressed name)  TYPE(2)  CLASS(2)  TTL(4)  RDLENGTH(2)  RDATA(RDLENGTH)
func unpackRR(msg []byte, off int) (ResourceRecord, int, error) {
	name, pos, err := decodeName(msg, off)
	if err != nil {
		return ResourceRecord{}, 0, err
	}
	// We need 10 fixed bytes (TYPE, CLASS, TTL, RDLENGTH) after the name.
	if pos+10 > len(msg) {
		return ResourceRecord{}, 0, fmt.Errorf("record header runs past end of message")
	}
	rr := ResourceRecord{
		Name:  name,
		Type:  binary.BigEndian.Uint16(msg[pos:]),
		Class: binary.BigEndian.Uint16(msg[pos+2:]),
		TTL:   binary.BigEndian.Uint32(msg[pos+4:]),
	}
	rdlen := int(binary.BigEndian.Uint16(msg[pos+8:]))
	pos += 10
	if pos+rdlen > len(msg) {
		return ResourceRecord{}, 0, fmt.Errorf("RDATA (%d bytes) runs past end of message", rdlen)
	}
	rr.RData = msg[pos : pos+rdlen]
	rr.Data = decodeRData(msg, rr.Type, pos, rdlen)
	return rr, pos + rdlen, nil
}

// decodeRData turns the raw RDATA into a readable string based on the record
// type. The crucial subtlety: names inside RDATA (NS, CNAME, MX) can ALSO use
// compression pointers back into the message, so we must decode them against
// the full msg, not just the RData slice.
func decodeRData(msg []byte, typ uint16, off, rdlen int) string {
	switch typ {
	case TypeA:
		if rdlen == 4 {
			b := msg[off : off+4]
			return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
		}
	case TypeAAAA:
		if rdlen == 16 {
			return formatIPv6(msg[off : off+16])
		}
	case TypeNS, TypeCNAME:
		if name, _, err := decodeName(msg, off); err == nil {
			return name
		}
	case TypeMX:
		if rdlen >= 3 {
			pref := binary.BigEndian.Uint16(msg[off:])
			if host, _, err := decodeName(msg, off+2); err == nil {
				return fmt.Sprintf("%d %s", pref, host)
			}
		}
	}
	return fmt.Sprintf("%x", msg[off:off+rdlen]) // unknown: show hex
}

// formatIPv6 renders 16 bytes as a colon-separated IPv6 address. We keep it
// simple (no "::" zero-compression) because readability, not canonical form, is
// the goal here.
func formatIPv6(b []byte) string {
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = fmt.Sprintf("%x", binary.BigEndian.Uint16(b[i*2:]))
	}
	return strings.Join(parts, ":")
}

// ---------------------------------------------------------------------------
// Whole message: build a query, parse a response.
// ---------------------------------------------------------------------------

// Message is a fully decoded DNS message.
type Message struct {
	Header     Header
	Questions  []Question
	Answers    []ResourceRecord
	Authority  []ResourceRecord
	Additional []ResourceRecord
}

// buildQuery assembles the raw bytes of a standard query for one name/type.
// recursionDesired sets the RD bit: true when we ask a recursive resolver to do
// the work for us; false when we drive the delegation walk ourselves.
func buildQuery(id uint16, name string, qtype uint16, recursionDesired bool) []byte {
	h := Header{ID: id, QDCount: 1}
	if recursionDesired {
		h.Flags |= flagRD
	}
	buf := make([]byte, 0, 64)
	buf = h.pack(buf)
	q := Question{Name: name, Type: qtype, Class: ClassIN}
	return q.pack(buf)
}

// parseMessage decodes a complete DNS message from msg.
//
// 🐍 Note how every section reuses the SAME running offset: questions advance
// it, then answers continue from there, then authority, then additional. The
// compression pointers inside later sections refer back into the bytes we have
// already walked past — which is exactly why decodeName always takes the full
// message plus an offset.
func parseMessage(msg []byte) (*Message, error) {
	h, err := unpackHeader(msg)
	if err != nil {
		return nil, err
	}
	m := &Message{Header: h}
	off := 12

	for i := 0; i < int(h.QDCount); i++ {
		name, pos, err := decodeName(msg, off)
		if err != nil {
			return nil, fmt.Errorf("question %d: %w", i, err)
		}
		if pos+4 > len(msg) {
			return nil, fmt.Errorf("question %d: truncated type/class", i)
		}
		m.Questions = append(m.Questions, Question{
			Name:  name,
			Type:  binary.BigEndian.Uint16(msg[pos:]),
			Class: binary.BigEndian.Uint16(msg[pos+2:]),
		})
		off = pos + 4
	}

	readRRs := func(count int, label string) ([]ResourceRecord, error) {
		recs := make([]ResourceRecord, 0, count)
		for i := 0; i < count; i++ {
			rr, pos, err := unpackRR(msg, off)
			if err != nil {
				return nil, fmt.Errorf("%s record %d: %w", label, i, err)
			}
			recs = append(recs, rr)
			off = pos
		}
		return recs, nil
	}

	if m.Answers, err = readRRs(int(h.ANCount), "answer"); err != nil {
		return nil, err
	}
	if m.Authority, err = readRRs(int(h.NSCount), "authority"); err != nil {
		return nil, err
	}
	if m.Additional, err = readRRs(int(h.ARCount), "additional"); err != nil {
		return nil, err
	}
	return m, nil
}

// typeName maps a numeric record type to its mnemonic for printing / CLI input.
func typeName(t uint16) string {
	switch t {
	case TypeA:
		return "A"
	case TypeNS:
		return "NS"
	case TypeCNAME:
		return "CNAME"
	case TypeMX:
		return "MX"
	case TypeAAAA:
		return "AAAA"
	default:
		return fmt.Sprintf("TYPE%d", t)
	}
}

// parseType converts a CLI string like "A" or "aaaa" into its numeric type.
func parseType(s string) (uint16, error) {
	switch strings.ToUpper(s) {
	case "A":
		return TypeA, nil
	case "NS":
		return TypeNS, nil
	case "CNAME":
		return TypeCNAME, nil
	case "MX":
		return TypeMX, nil
	case "AAAA":
		return TypeAAAA, nil
	default:
		return 0, fmt.Errorf("unsupported record type %q (try A, AAAA, NS, CNAME, MX)", s)
	}
}

// rcodeText gives a readable name for a response code.
func rcodeText(rcode int) string {
	switch rcode {
	case 0:
		return "NOERROR"
	case 1:
		return "FORMERR"
	case 2:
		return "SERVFAIL"
	case 3:
		return "NXDOMAIN"
	case 4:
		return "NOTIMP"
	case 5:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", rcode)
	}
}
