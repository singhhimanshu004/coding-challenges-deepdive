package main

// DNS WIRE FORMAT (RFC 1035), self-contained.
//
// This file is adapted from the sibling challenge phase-04-networking/dns-resolver.
// The forwarder is a SERVER, so it needs far less than a full resolver: it only
// has to (1) read the QUESTION out of a client's query to build a cache key, and
// (2) read the answer RRs out of the upstream's reply to find the minimum TTL.
// We re-implement the parsing here (its own module) so the challenge stays
// independent — nothing is imported from the resolver.
//
// 🐍➡️🐹 Python-dev orientation:
//   - A []byte is Go's mutable byte buffer. Think `bytearray`. A string is an
//     immutable view of bytes; convert with []byte(s) / string(b).
//   - encoding/binary is Go's `struct.pack`/`struct.unpack`. DNS is BIG-ENDIAN
//     ("network byte order"), so we always use binary.BigEndian.
//   - There are no classes; we attach methods to plain structs via a receiver,
//     e.g. func (h Header) pack(). That's a function whose first arg is the value.

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// DNS resource-record TYPE values (the small subset we name for logging).
const (
	TypeA     uint16 = 1  // IPv4 address
	TypeNS    uint16 = 2  // authoritative name server
	TypeCNAME uint16 = 5  // canonical name (an alias)
	TypeMX    uint16 = 15 // mail exchange
	TypeAAAA  uint16 = 28 // IPv6 address
)

// ClassIN is the only DNS class anyone uses today: the Internet class.
const ClassIN uint16 = 1

// Header flag bits inside the 16-bit flags word (RFC 1035 §4.1.1).
const (
	flagQR = 1 << 15 // 0 = query, 1 = response
	flagRD = 1 << 8  // recursion desired (set by the client)
)

// Header mirrors the fixed 12-byte DNS header: six 16-bit big-endian integers.
type Header struct {
	ID      uint16 // request id, echoed in the response so queries/replies match
	Flags   uint16 // QR/Opcode/AA/TC/RD/RA/Z/RCODE packed together
	QDCount uint16 // number of entries in the question section
	ANCount uint16 // number of resource records in the answer section
	NSCount uint16 // number of name-server (authority) records
	ARCount uint16 // number of additional records
}

// pack appends the header as 12 big-endian bytes to buf.
//
// 🐍 binary.BigEndian.PutUint16 is struct.pack(">H", n): write the two bytes
// most-significant-first.
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

// Question is one entry in the question section: the trio that uniquely
// identifies "what is being asked" — and exactly the key we cache responses by.
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

// encodeName writes a domain name in DNS label form.
//
// A name is NOT "www.example.com\0". Each dot-separated label is length-prefixed
// and a zero-length label (a single 0x00) terminates the name:
//
//	www.example.com -> 3 'w' 'w' 'w' 7 'e' 'x' 'a' 'm' 'p' 'l' 'e' 3 'c' 'o' 'm' 0
func encodeName(buf []byte, name string) []byte {
	name = strings.TrimSuffix(name, ".")
	if name != "" {
		for _, label := range strings.Split(name, ".") {
			if len(label) > 63 {
				label = label[:63]
			}
			buf = append(buf, byte(len(label)))
			buf = append(buf, label...)
		}
	}
	return append(buf, 0x00)
}

// decodeName reads a (possibly compressed) domain name starting at offset off in
// the FULL message msg. It returns the dotted name and the offset of the first
// byte AFTER the name in the original stream.
//
// ── DNS NAME COMPRESSION ──────────────────────────────────────────────────
// To save space a name can end with a POINTER to a name seen earlier in the
// same message. A length byte whose top two bits are set (byte & 0xC0 == 0xC0)
// is not a length: that byte plus the next form a 14-bit OFFSET from the start
// of the message. We jump there and keep reading. The offset we RETURN is the
// position just past the 2-byte pointer in the original stream, not the jump
// target. We cap jumps so a malicious loop can't hang us.
func decodeName(msg []byte, off int) (string, int, error) {
	var labels []string
	pos := off
	returnOff := -1
	jumps := 0

	for {
		if pos >= len(msg) {
			return "", 0, fmt.Errorf("name runs past end of message at %d", pos)
		}
		b := msg[pos]

		switch {
		case b&0xC0 == 0xC0:
			if pos+1 >= len(msg) {
				return "", 0, fmt.Errorf("truncated compression pointer at %d", pos)
			}
			if returnOff == -1 {
				returnOff = pos + 2
			}
			ptr := int(binary.BigEndian.Uint16(msg[pos:]) & 0x3FFF)
			jumps++
			if jumps > 64 {
				return "", 0, fmt.Errorf("too many compression pointers (loop?)")
			}
			pos = ptr

		case b == 0x00:
			pos++
			if returnOff == -1 {
				returnOff = pos
			}
			return strings.Join(labels, "."), returnOff, nil

		default:
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

// ResourceRecord is one parsed RR. For the forwarder we mostly care about TTL,
// but we keep Type/Name/Data so verbose logging can show something readable.
type ResourceRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  string
}

// unpackRR reads one resource record starting at offset off and returns the
// record plus the offset just past it.
//
// RR layout (RFC 1035 §3.2.1):
//
//	NAME(compressed) TYPE(2) CLASS(2) TTL(4) RDLENGTH(2) RDATA(RDLENGTH)
func unpackRR(msg []byte, off int) (ResourceRecord, int, error) {
	name, pos, err := decodeName(msg, off)
	if err != nil {
		return ResourceRecord{}, 0, err
	}
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
	rr.Data = decodeRData(msg, rr.Type, pos, rdlen)
	return rr, pos + rdlen, nil
}

// decodeRData renders a few common record types for logging. Anything else is
// shown as hex. (The forwarder relays raw bytes verbatim; this is only cosmetic.)
func decodeRData(msg []byte, typ uint16, off, rdlen int) string {
	switch typ {
	case TypeA:
		if rdlen == 4 {
			b := msg[off : off+4]
			return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
		}
	case TypeNS, TypeCNAME:
		if name, _, err := decodeName(msg, off); err == nil {
			return name
		}
	}
	return fmt.Sprintf("%x", msg[off:off+rdlen])
}

// Message is a decoded DNS message (only the sections the forwarder needs).
type Message struct {
	Header    Header
	Questions []Question
	Answers   []ResourceRecord
}

// parseMessage decodes the header, the question section, and the answer section.
// It deliberately stops after answers: the forwarder never inspects authority or
// additional records, and parsing only what we need keeps the hot path cheap.
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

	for i := 0; i < int(h.ANCount); i++ {
		rr, pos, err := unpackRR(msg, off)
		if err != nil {
			return nil, fmt.Errorf("answer %d: %w", i, err)
		}
		m.Answers = append(m.Answers, rr)
		off = pos
	}
	return m, nil
}

// minTTL returns the smallest TTL across the answer records, in seconds, plus a
// flag reporting whether any cacheable answer was present. A reply with no
// answers (NXDOMAIN, empty NOERROR) returns (0, false) and is NOT cached here.
func (m *Message) minTTL() (uint32, bool) {
	if len(m.Answers) == 0 {
		return 0, false
	}
	min := m.Answers[0].TTL
	for _, rr := range m.Answers[1:] {
		if rr.TTL < min {
			min = rr.TTL
		}
	}
	return min, true
}

// typeName maps a numeric record type to its mnemonic, for log lines.
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
