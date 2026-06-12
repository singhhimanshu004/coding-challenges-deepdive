package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// packetSize is the fixed wire size of an NTP (and SNTP) message. Every NTP
// packet — request and response — is exactly 48 bytes. There are optional
// authentication fields after byte 48, but we never send or expect them.
const packetSize = 48

// ntpEpochOffset is the number of seconds between the two epochs we have to
// bridge:
//
//	NTP epoch:  1900-01-01 00:00:00 UTC
//	Unix epoch: 1970-01-01 00:00:00 UTC
//
// That gap is 70 years. 70 * 365 days = 25550 days, plus 17 leap days between
// 1900 and 1970, = 25567 days * 86400 seconds/day = 2,208,988,800 seconds.
// To convert an NTP timestamp to Unix time we SUBTRACT this constant; to go the
// other way we ADD it.
const ntpEpochOffset = 2208988800

// ntpTime is the 64-bit fixed-point timestamp format used throughout NTP. The
// high 32 bits count whole seconds since the NTP epoch (1900). The low 32 bits
// are the FRACTION of a second, interpreted as Fraction / 2^32. So a Fraction
// of 0x80000000 (half of 2^32) means exactly 0.5 seconds.
//
// 🐍 In Python you'd probably reach for struct.unpack(">II", ...) and do the
// math by hand. Here the two uint32 fields map 1:1 onto the 8 wire bytes, and
// encoding/binary reads them for us.
type ntpTime struct {
	Seconds  uint32
	Fraction uint32
}

// packet mirrors the 48-byte NTP message exactly, field for field, in wire
// order. Because every field is a fixed-size integer (or an ntpTime, which is
// two uint32s), encoding/binary can read the whole struct in one call — the Go
// struct layout IS the wire layout.
//
// Byte map (offsets):
//
//	 0      Settings   LI(2) | VN(3) | Mode(3)
//	 1      Stratum
//	 2      Poll
//	 3      Precision
//	 4..7   RootDelay
//	 8..11  RootDispersion
//	12..15  ReferenceID
//	16..23  RefTimestamp   (reference)
//	24..31  OrigTimestamp  T1 — copied back from our request
//	32..39  RecvTimestamp  T2 — when the server received our request
//	40..47  TxTimestamp    T3 — when the server sent its reply
type packet struct {
	Settings       uint8 // LI | VN | Mode, packed into one byte
	Stratum        uint8
	Poll           int8
	Precision      int8
	RootDelay      uint32
	RootDispersion uint32
	ReferenceID    uint32
	RefTimestamp   ntpTime
	OrigTimestamp  ntpTime // T1
	RecvTimestamp  ntpTime // T2
	TxTimestamp    ntpTime // T3
}

// NTP modes. Mode 3 means "client", which is what we always send. The server
// replies with mode 4 ("server").
const (
	modeClient uint8 = 3
)

// buildRequest constructs the 48-byte client request. The trick of an NTP
// client request is that almost every byte is ZERO — we only need to set the
// first byte so the server knows the protocol version and that we are a client.
//
// The first byte packs three subfields, most-significant bits first:
//
//	LI  (Leap Indicator)  bits 7..6  -> 0 (no warning)
//	VN  (Version Number)  bits 5..3  -> 3 or 4
//	Mode                  bits 2..0  -> 3 (client)
//
// 🐍 This is bit packing: `version << 3` slides the 3 version bits into
// position 5..3, then OR-ing in the mode (which occupies bits 2..0) glues them
// together. Same idea as Python's `(version << 3) | mode`.
func buildRequest(version uint8) []byte {
	b := make([]byte, packetSize)
	b[0] = (0 << 6) | (version << 3) | modeClient
	return b
}

// parseResponse decodes 48 raw bytes from the wire into a packet. binary.Read
// walks the struct field by field, pulling the right number of bytes for each
// and interpreting them as big-endian (network byte order). One call fills the
// entire struct.
func parseResponse(b []byte) (*packet, error) {
	if len(b) < packetSize {
		return nil, fmt.Errorf("short NTP response: got %d bytes, want at least %d", len(b), packetSize)
	}
	var p packet
	if err := binary.Read(bytes.NewReader(b), binary.BigEndian, &p); err != nil {
		return nil, fmt.Errorf("decoding NTP packet: %w", err)
	}
	return &p, nil
}

// toTime converts a fixed-point NTP timestamp into a Go time.Time in UTC.
//
//   - Whole seconds: subtract ntpEpochOffset to rebase from 1900 to 1970.
//   - Fraction: it's a fraction of a second scaled by 2^32, so to get
//     nanoseconds we compute fraction/2^32 * 1e9. We do it as
//     (fraction * 1e9) >> 32 to stay in integer math (>>32 divides by 2^32).
//
// A zero timestamp (both halves 0) is "no value" in NTP, so we return the zero
// time.Time for it.
func (t ntpTime) toTime() time.Time {
	if t.Seconds == 0 && t.Fraction == 0 {
		return time.Time{}
	}
	secs := int64(t.Seconds) - ntpEpochOffset
	nanos := (int64(t.Fraction) * 1_000_000_000) >> 32
	return time.Unix(secs, nanos).UTC()
}

// timeToNTP is the inverse of toTime: it converts a Go time.Time into the
// fixed-point NTP format. We use it to stamp our request's transmit time, which
// well-behaved servers echo back as our originate timestamp (T1).
func timeToNTP(t time.Time) ntpTime {
	secs := uint32(t.Unix() + ntpEpochOffset)
	frac := uint32((int64(t.Nanosecond()) << 32) / 1_000_000_000)
	return ntpTime{Seconds: secs, Fraction: frac}
}

// clockMetrics computes the two numbers an NTP client cares about from the four
// timestamps in a transaction:
//
//	T1 = originate    (local clock, just before we send)
//	T2 = receive      (server clock, when it got our request)
//	T3 = transmit     (server clock, when it sent the reply)
//	T4 = destination  (local clock, just after we got the reply)
//
// offset = ((T2 - T1) + (T3 - T4)) / 2
//
//	How far our clock is from the server's. The two halves estimate the
//	error on the outbound and inbound legs; averaging them cancels the
//	network travel time, assuming the path is symmetric.
//
// delay = (T4 - T1) - (T3 - T2)
//
//	The round-trip network time. (T4 - T1) is the total time we observed;
//	subtracting (T3 - T2), the time the server spent thinking, leaves only
//	the time spent on the wire.
//
// 🐍 time.Duration is just an int64 of nanoseconds with nice printing, so
// ordinary +, -, and / arithmetic works exactly like you'd expect.
func clockMetrics(t1, t2, t3, t4 time.Time) (offset, delay time.Duration) {
	offset = ((t2.Sub(t1)) + (t3.Sub(t4))) / 2
	delay = (t4.Sub(t1)) - (t3.Sub(t2))
	return offset, delay
}
