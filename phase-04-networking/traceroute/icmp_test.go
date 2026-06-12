package main

import (
	"net"
	"testing"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// TestBuildEchoRequestRoundTrip builds an echo request and parses it back,
// confirming our serialiser produces bytes a decoder accepts. No socket needed —
// this is the whole point of isolating the byte logic.
func TestBuildEchoRequestRoundTrip(t *testing.T) {
	b, err := buildEchoRequest(0x1234, 7, []byte("hello"))
	if err != nil {
		t.Fatalf("buildEchoRequest: %v", err)
	}

	msg, err := icmp.ParseMessage(protocolICMP, b)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if msg.Type != ipv4.ICMPTypeEcho {
		t.Fatalf("type = %v, want echo request", msg.Type)
	}
	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		t.Fatalf("body type = %T, want *icmp.Echo", msg.Body)
	}
	if echo.ID != 0x1234 {
		t.Errorf("ID = %d, want %d", echo.ID, 0x1234)
	}
	if echo.Seq != 7 {
		t.Errorf("Seq = %d, want 7", echo.Seq)
	}
	if string(echo.Data) != "hello" {
		t.Errorf("Data = %q, want %q", echo.Data, "hello")
	}
}

// marshal is a tiny helper that turns an icmp.Message into wire bytes for tests,
// failing the test on error.
func marshal(t *testing.T, m icmp.Message) []byte {
	t.Helper()
	b, err := m.Marshal(nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestParseICMPReplyClassification feeds crafted ICMP packets through the parser
// and asserts each is classified correctly. This covers the three outcomes
// traceroute hinges on plus the "ignore this" case — all from synthetic bytes,
// so it runs offline and without privileges.
func TestParseICMPReplyClassification(t *testing.T) {
	// A plausible inner payload for the error messages (original IP header etc.).
	// The contents don't matter to our classifier; only the outer type does.
	inner := make([]byte, 28)

	tests := []struct {
		name    string
		bytes   []byte
		want    replyKind
		wantSeq int
	}{
		{
			name: "time exceeded => intermediate hop",
			bytes: marshal(t, icmp.Message{
				Type: ipv4.ICMPTypeTimeExceeded,
				Code: 0,
				Body: &icmp.TimeExceeded{Data: inner},
			}),
			want: replyTimeExceeded,
		},
		{
			name: "echo reply => destination reached",
			bytes: marshal(t, icmp.Message{
				Type: ipv4.ICMPTypeEchoReply,
				Code: 0,
				Body: &icmp.Echo{ID: 1, Seq: 42, Data: []byte("x")},
			}),
			want:    replyEchoReply,
			wantSeq: 42,
		},
		{
			name: "destination unreachable => terminal",
			bytes: marshal(t, icmp.Message{
				Type: ipv4.ICMPTypeDestinationUnreachable,
				Code: 3, // port unreachable
				Body: &icmp.DstUnreach{Data: inner},
			}),
			want: replyDestUnreachable,
		},
		{
			name: "echo request (not a reply) => ignored",
			bytes: marshal(t, icmp.Message{
				Type: ipv4.ICMPTypeEcho,
				Code: 0,
				Body: &icmp.Echo{ID: 1, Seq: 1, Data: []byte("x")},
			}),
			want: replyOther,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, seq, err := parseICMPReply(tc.bytes)
			if err != nil {
				t.Fatalf("parseICMPReply: %v", err)
			}
			if kind != tc.want {
				t.Errorf("kind = %v, want %v", kind, tc.want)
			}
			if tc.want == replyEchoReply && seq != tc.wantSeq {
				t.Errorf("seq = %d, want %d", seq, tc.wantSeq)
			}
		})
	}
}

// TestParseICMPReplyGarbage confirms we surface an error for bytes that aren't a
// valid ICMP message, instead of misclassifying them.
func TestParseICMPReplyGarbage(t *testing.T) {
	if _, _, err := parseICMPReply([]byte{0x01}); err == nil {
		t.Fatal("expected error for truncated/invalid ICMP bytes, got nil")
	}
}

// TestReplyKindTerminal documents which outcomes end the trace.
func TestReplyKindTerminal(t *testing.T) {
	cases := map[replyKind]bool{
		replyOther:           false,
		replyTimeExceeded:    false,
		replyEchoReply:       true,
		replyDestUnreachable: true,
	}
	for k, want := range cases {
		if got := k.terminal(); got != want {
			t.Errorf("%v.terminal() = %v, want %v", k, got, want)
		}
	}
}

// ipAddr is a small helper for building net.Addr values in tests.
func ipAddr(s string) net.Addr { return &net.IPAddr{IP: net.ParseIP(s)} }
