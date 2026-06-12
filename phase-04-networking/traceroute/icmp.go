package main

// icmp.go isolates the two pieces of pure byte-wrangling that make traceroute
// work: turning a probe into ICMP echo-request bytes, and classifying the raw
// bytes that come back. Keeping these here — with no socket, no network, no
// privileges — is exactly what lets us unit-test the protocol logic offline.

import (
	"fmt"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// protocolICMP is the IANA protocol number for ICMP for IPv4 (1). icmp.ParseMessage
// needs it to know how to decode the bytes the kernel hands us.
//
// 🐍 In Python's `socket` you'd pass socket.IPPROTO_ICMP (also 1). Same constant,
// different spelling.
const protocolICMP = 1

// replyKind is our small classification of "what did this ICMP packet mean for
// the hop we're probing?". Traceroute only cares about three outcomes.
//
// 🐍➡️🐹 Go has no enum keyword. The idiom is a named integer type plus a block
// of iota constants — iota auto-increments 0,1,2,… down the const block. This is
// the Go equivalent of Python's enum.IntEnum.
type replyKind int

const (
	// replyOther means "an ICMP packet we don't act on" (e.g. a stray echo
	// request, an unknown type). Treated like no useful answer.
	replyOther replyKind = iota
	// replyTimeExceeded is the heart of traceroute: a router decremented our
	// packet's TTL to zero and sent back "Time Exceeded". The packet's SOURCE
	// address is that router — one hop on the path.
	replyTimeExceeded
	// replyEchoReply means our probe reached the DESTINATION and it answered our
	// ping. The trace is complete.
	replyEchoReply
	// replyDestUnreachable means we reached the destination (or its last router)
	// but the target/port is unreachable. For UDP-probe traceroute this is the
	// normal "we arrived" signal; we treat it as terminal too.
	replyDestUnreachable
)

func (k replyKind) String() string {
	switch k {
	case replyTimeExceeded:
		return "time-exceeded"
	case replyEchoReply:
		return "echo-reply"
	case replyDestUnreachable:
		return "dest-unreachable"
	default:
		return "other"
	}
}

// terminal reports whether this kind of reply means the trace should stop: the
// probe reached the destination.
func (k replyKind) terminal() bool {
	return k == replyEchoReply || k == replyDestUnreachable
}

// buildEchoRequest serialises an ICMP echo (ping) request into wire bytes.
//
// id   — identifier field; lets us match replies to this run. On unprivileged
//
//	datagram sockets the kernel may OVERWRITE this with the socket's port,
//	so we don't rely on it for matching (see traceroute.go), but we still set
//	it because a well-formed packet needs one.
//
// seq  — sequence number; we bump it per probe so successive pings differ.
// payload — arbitrary bytes echoed back by the destination; we send a short tag.
//
// 🐍 This is the moral equivalent of building a struct with struct.pack(...) in
// Python, except x/net/icmp does the checksum and field layout for us. The
// returned []byte is exactly what goes on the wire.
func buildEchoRequest(id, seq int, payload []byte) ([]byte, error) {
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho, // type 8 = echo request
		Code: 0,
		Body: &icmp.Echo{
			ID:   id & 0xffff, // these fields are 16-bit on the wire
			Seq:  seq & 0xffff,
			Data: payload,
		},
	}
	// Marshal computes the checksum and lays out the bytes. The nil arg is the
	// optional pseudo-header used only for ICMPv6; IPv4 ICMP doesn't need it.
	b, err := msg.Marshal(nil)
	if err != nil {
		return nil, fmt.Errorf("marshal echo request: %w", err)
	}
	return b, nil
}

// parseICMPReply decodes raw ICMP bytes and classifies them for traceroute.
//
// It returns the replyKind plus, when the kind is replyEchoReply, the sequence
// number echoed back (so callers can match a reply to the probe that triggered
// it). The responding router's IP is NOT in here — it comes from the socket's
// recvfrom peer address, which the caller supplies — so this function stays a
// pure bytes-in / meaning-out transform that's trivial to unit test.
//
// 🐍 Think of this as the "parse the response" half of a request/response pair,
// like reading the status line + headers after you've written an HTTP request.
func parseICMPReply(b []byte) (kind replyKind, echoSeq int, err error) {
	msg, err := icmp.ParseMessage(protocolICMP, b)
	if err != nil {
		return replyOther, 0, fmt.Errorf("parse icmp message: %w", err)
	}

	switch body := msg.Body.(type) {
	case *icmp.TimeExceeded:
		// A router on the path. (body.Data holds the IP header + first bytes of
		// our original probe; we don't need to dig into it for this challenge.)
		return replyTimeExceeded, 0, nil
	case *icmp.DstUnreach:
		return replyDestUnreachable, 0, nil
	case *icmp.Echo:
		// On a "udp4" ICMP socket the kernel delivers echo *replies* to us, but
		// x/net/icmp decodes both echo request and reply into *icmp.Echo. We
		// distinguish by the message Type, which is the authoritative field.
		if msg.Type == ipv4.ICMPTypeEchoReply {
			return replyEchoReply, body.Seq, nil
		}
		return replyOther, body.Seq, nil
	default:
		return replyOther, 0, nil
	}
}
