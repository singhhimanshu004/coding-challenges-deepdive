package main

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// TestBuildRequest verifies the ENCODE side: a request is exactly 48 bytes,
// the first byte packs LI/VN/Mode correctly, and every other byte is zero.
func TestBuildRequest(t *testing.T) {
	tests := []struct {
		name      string
		version   uint8
		wantFirst byte
	}{
		// LI=0, Mode=3 in every case; only the version bits (5..3) change.
		{"version 3", 3, 0b00_011_011}, // 0x1B
		{"version 4", 4, 0b00_100_011}, // 0x23
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := buildRequest(tc.version)

			if len(req) != packetSize {
				t.Fatalf("request length = %d, want %d", len(req), packetSize)
			}
			if req[0] != tc.wantFirst {
				t.Errorf("first byte = %#08b, want %#08b", req[0], tc.wantFirst)
			}

			// Decode the first byte back into its subfields and check each.
			li := req[0] >> 6
			vn := (req[0] >> 3) & 0b111
			mode := req[0] & 0b111
			if li != 0 {
				t.Errorf("LI = %d, want 0", li)
			}
			if vn != tc.version {
				t.Errorf("VN = %d, want %d", vn, tc.version)
			}
			if mode != modeClient {
				t.Errorf("Mode = %d, want %d", mode, modeClient)
			}

			// All remaining bytes must be zero for a bare client request.
			for i := 1; i < len(req); i++ {
				if req[i] != 0 {
					t.Errorf("byte %d = %d, want 0", i, req[i])
				}
			}
		})
	}
}

// TestNTPTimeToTime verifies the DECODE side: the 64-bit fixed-point NTP
// timestamp converts to the correct Unix time, including the 1900->1970 epoch
// shift and the fractional-seconds math. No network involved — crafted values.
func TestNTPTimeToTime(t *testing.T) {
	tests := []struct {
		name      string
		ts        ntpTime
		wantUnix  int64 // expected Unix seconds
		wantNanos int   // expected nanosecond component
		wantZero  bool  // expect the zero time.Time
	}{
		{
			// NTP seconds == the epoch offset means "Unix time 0", i.e.
			// 1970-01-01T00:00:00Z. This is the cleanest possible epoch check.
			name:     "unix epoch",
			ts:       ntpTime{Seconds: ntpEpochOffset, Fraction: 0},
			wantUnix: 0,
		},
		{
			// One second past the Unix epoch.
			name:     "one second after unix epoch",
			ts:       ntpTime{Seconds: ntpEpochOffset + 1, Fraction: 0},
			wantUnix: 1,
		},
		{
			// Fraction = 0x80000000 = 2^31 = half of 2^32 -> exactly 0.5s.
			name:      "half-second fraction",
			ts:        ntpTime{Seconds: ntpEpochOffset, Fraction: 0x80000000},
			wantUnix:  0,
			wantNanos: 500_000_000,
		},
		{
			// Fraction = 0x40000000 = 2^30 = quarter of 2^32 -> 0.25s.
			name:      "quarter-second fraction",
			ts:        ntpTime{Seconds: ntpEpochOffset, Fraction: 0x40000000},
			wantUnix:  0,
			wantNanos: 250_000_000,
		},
		{
			// A zero timestamp is "unset" in NTP and must map to time.Time{}.
			name:     "zero is unset",
			ts:       ntpTime{Seconds: 0, Fraction: 0},
			wantZero: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ts.toTime()

			if tc.wantZero {
				if !got.IsZero() {
					t.Fatalf("toTime() = %v, want zero time", got)
				}
				return
			}

			if got.Unix() != tc.wantUnix {
				t.Errorf("Unix seconds = %d, want %d", got.Unix(), tc.wantUnix)
			}
			if got.Nanosecond() != tc.wantNanos {
				t.Errorf("nanoseconds = %d, want %d", got.Nanosecond(), tc.wantNanos)
			}
		})
	}
}

// TestTimeToNTPRoundTrip checks that timeToNTP and toTime are inverses to
// within the resolution of the fixed-point format (sub-nanosecond rounding is
// unavoidable, so we allow a tiny tolerance).
func TestTimeToNTPRoundTrip(t *testing.T) {
	orig := time.Date(2024, 3, 15, 10, 30, 45, 123_456_789, time.UTC)
	got := timeToNTP(orig).toTime()

	diff := got.Sub(orig)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Microsecond {
		t.Errorf("round trip drifted by %v (got %v, want %v)", diff, got, orig)
	}
}

// TestParseResponse builds a 48-byte packet with binary.Write, parses it back,
// and confirms the fields land in the right place (big-endian, correct offsets).
func TestParseResponse(t *testing.T) {
	want := packet{
		Settings:      0b00_100_100, // LI=0, VN=4, Mode=4 (server reply)
		Stratum:       2,
		RecvTimestamp: ntpTime{Seconds: ntpEpochOffset + 100, Fraction: 0},
		TxTimestamp:   ntpTime{Seconds: ntpEpochOffset + 101, Fraction: 0x80000000},
	}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, &want); err != nil {
		t.Fatalf("crafting packet: %v", err)
	}
	if buf.Len() != packetSize {
		t.Fatalf("crafted packet is %d bytes, want %d", buf.Len(), packetSize)
	}

	got, err := parseResponse(buf.Bytes())
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}

	if got.Settings != want.Settings {
		t.Errorf("Settings = %#08b, want %#08b", got.Settings, want.Settings)
	}
	if got.Stratum != want.Stratum {
		t.Errorf("Stratum = %d, want %d", got.Stratum, want.Stratum)
	}
	if got.RecvTimestamp != want.RecvTimestamp {
		t.Errorf("RecvTimestamp = %+v, want %+v", got.RecvTimestamp, want.RecvTimestamp)
	}
	if got.TxTimestamp != want.TxTimestamp {
		t.Errorf("TxTimestamp = %+v, want %+v", got.TxTimestamp, want.TxTimestamp)
	}
}

// TestParseResponseShort confirms we reject undersized buffers instead of
// reading past the end.
func TestParseResponseShort(t *testing.T) {
	if _, err := parseResponse(make([]byte, 10)); err == nil {
		t.Fatal("expected error for short packet, got nil")
	}
}

// TestClockMetrics checks the offset and delay formulas against hand-computed
// values. We pick timestamps where the answer is obvious.
func TestClockMetrics(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Construct a transaction where the server clock is 10s AHEAD of ours and
	// the network legs are symmetric (1s each way).
	//
	//	T1 = 0s   (local send)
	//	T2 = 11s  (server receive: 1s travel + 10s offset)
	//	T3 = 11s  (server transmit: assume zero server processing)
	//	T4 = 2s   (local receive: 2s after send)
	t1 := base
	t2 := base.Add(11 * time.Second)
	t3 := base.Add(11 * time.Second)
	t4 := base.Add(2 * time.Second)

	offset, delay := clockMetrics(t1, t2, t3, t4)

	// offset = ((11-0) + (11-2)) / 2 = (11 + 9) / 2 = 10s
	if want := 10 * time.Second; offset != want {
		t.Errorf("offset = %v, want %v", offset, want)
	}
	// delay = (2-0) - (11-11) = 2s
	if want := 2 * time.Second; delay != want {
		t.Errorf("delay = %v, want %v", delay, want)
	}
}
