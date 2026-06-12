package main

// This file owns the NETWORK side: opening a raw UDP socket, sending a query
// blob, and reading the reply. It also implements the two resolution modes:
//
//   1. Recursive-resolver mode (default): ask a configured resolver such as
//      8.8.8.8:53 with the RD (recursion-desired) bit set and let IT do the
//      legwork. One question, one answer.
//
//   2. Iterative mode (--trace): act like a real resolver ourselves. Start at a
//      root server, ask with RD=0, and follow the NS REFERRALS down the
//      delegation chain (root -> .com -> example.com -> the answer). This is
//      "recursive resolution from the root" as seen from the outside.
//
// 🐍➡️🐹 net.DialUDP / net.UDPConn is Python's
//   s = socket.socket(AF_INET, SOCK_DGRAM); s.connect((ip, 53))
// A net.Conn is just something with Read([]byte) and Write([]byte). UDP is
// connectionless, but Dial lets us Write/Read without repeating the address.

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

// rootServers are the well-known IPv4 addresses of a few DNS root servers. The
// iterative walk begins by asking one of these "where do I find .com?".
var rootServers = []string{
	"198.41.0.4",     // a.root-servers.net
	"199.9.14.201",   // b.root-servers.net
	"192.33.4.12",    // c.root-servers.net
	"199.7.91.13",    // d.root-servers.net
	"192.203.230.10", // e.root-servers.net
}

// queryTimeout bounds how long we wait for a single UDP reply before giving up.
const queryTimeout = 5 * time.Second

// exchange sends one query to server (an "ip:port") over UDP and returns the
// parsed response. This is the lowest-level network primitive in the project.
//
// THE BIG IDEA: there is no "DNS library" doing anything here. We push the
// bytes we hand-encoded into the socket and parse the bytes that come back.
func exchange(server string, query []byte) (*Message, error) {
	// Resolve "ip:port" into a UDP address struct. (For our purposes server is
	// always already a literal IP, so this does no name lookup.)
	raddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("bad server address %q: %w", server, err)
	}

	// DialUDP opens the socket. Passing a nil local address lets the OS pick an
	// ephemeral source port for us. defer Close() is Go's `with`/finally.
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()

	// A read/write deadline guards against a server that never answers (UDP has
	// no connection to "drop", so without this we'd block forever).
	_ = conn.SetDeadline(time.Now().Add(queryTimeout))

	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("send to %s: %w", server, err)
	}

	// A DNS-over-UDP message is capped at 512 bytes historically; 1232 is a
	// safe modern EDNS-free buffer. One Read returns one whole datagram.
	buf := make([]byte, 1232)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read from %s: %w", server, err)
	}

	return parseMessage(buf[:n])
}

// newID returns a random 16-bit query ID. Matching this in the reply is a
// basic spoofing defence (and lets a resolver juggle many in-flight queries).
func newID() uint16 {
	return uint16(rand.Intn(1 << 16))
}

// resolveRecursive asks a recursive resolver (e.g. 8.8.8.8:53) to do all the
// work. We set RD=1 and trust the single response it returns.
func resolveRecursive(server, name string, qtype uint16) (*Message, error) {
	query := buildQuery(newID(), name, qtype, true)
	resp, err := exchange(server, query)
	if err != nil {
		return nil, err
	}
	if resp.Header.rcode() != 0 {
		return resp, fmt.Errorf("server returned %s", rcodeText(resp.Header.rcode()))
	}
	return resp, nil
}

// resolveIterative performs the delegation walk ourselves, starting from a root
// server. At each step we send a NON-recursive query (RD=0). The server either:
//
//   - returns the ANSWER (we're done), or
//   - returns a CNAME (we restart the walk for the alias target), or
//   - returns a REFERRAL: NS records naming the servers for a lower zone, often
//     with their IP addresses in the additional section ("glue"). We pick one
//     of those servers and ask again, one level deeper.
//
// trace, when non-nil, receives a human-readable line per hop (like dig +trace).
func resolveIterative(name string, qtype uint16, trace func(string)) (*Message, error) {
	servers := append([]string(nil), rootServers...)
	current := name

	for hop := 0; hop < 16; hop++ { // hard cap to guarantee termination
		server := net.JoinHostPort(servers[0], "53")
		if trace != nil {
			trace(fmt.Sprintf("hop %d: asking %s for %s %s", hop, servers[0], current, typeName(qtype)))
		}

		query := buildQuery(newID(), current, qtype, false)
		resp, err := exchange(server, query)
		if err != nil {
			// This server didn't answer; try the next candidate at this level.
			if len(servers) > 1 {
				servers = servers[1:]
				hop--
				continue
			}
			return nil, err
		}

		if resp.Header.rcode() == 3 {
			return resp, fmt.Errorf("%s: %s", current, rcodeText(3)) // NXDOMAIN
		}

		// 1) Did we get the answer we asked for?
		if hasType(resp.Answers, qtype) {
			if trace != nil {
				trace(fmt.Sprintf("hop %d: got answer from %s", hop, servers[0]))
			}
			return resp, nil
		}

		// 2) A CNAME alias? Follow it: restart the walk for the canonical name.
		if cname := firstOfType(resp.Answers, TypeCNAME); cname != "" {
			if trace != nil {
				trace(fmt.Sprintf("hop %d: CNAME %s -> %s, restarting from root", hop, current, cname))
			}
			current = cname
			servers = append([]string(nil), rootServers...)
			continue
		}

		// 3) A referral: collect the next-level name servers and their glue IPs.
		next := referralServers(resp)
		if len(next) == 0 {
			// No answer, no referral we can use — best effort, return what we got.
			return resp, nil
		}
		servers = next
	}
	return nil, fmt.Errorf("gave up after too many referrals resolving %s", name)
}

// referralServers extracts the IP addresses of the name servers a referral
// response points us to. It prefers GLUE (A records for the NS hosts, supplied
// in the additional section) so we don't have to recursively resolve the NS
// names ourselves. If no glue is present, it resolves the NS hostnames via a
// fresh iterative walk.
func referralServers(resp *Message) []string {
	// Names of the authoritative servers for the lower zone.
	var nsNames []string
	for _, rr := range resp.Authority {
		if rr.Type == TypeNS {
			nsNames = append(nsNames, rr.Data)
		}
	}
	if len(nsNames) == 0 {
		return nil
	}

	// Glue: A records in the additional section give us the NS hosts' IPs.
	glue := map[string]string{}
	for _, rr := range resp.Additional {
		if rr.Type == TypeA {
			glue[rr.Name] = rr.Data
		}
	}

	var ips []string
	for _, ns := range nsNames {
		if ip, ok := glue[ns]; ok {
			ips = append(ips, ip)
		}
	}
	if len(ips) > 0 {
		return ips
	}

	// No glue: resolve one NS hostname from the root to get an IP to continue.
	for _, ns := range nsNames {
		if r, err := resolveIterative(ns, TypeA, nil); err == nil {
			if ip := firstOfType(r.Answers, TypeA); ip != "" {
				return []string{ip}
			}
		}
	}
	return nil
}

// hasType reports whether any record in rrs has the given type.
func hasType(rrs []ResourceRecord, t uint16) bool {
	for _, rr := range rrs {
		if rr.Type == t {
			return true
		}
	}
	return false
}

// firstOfType returns the decoded Data of the first record of type t, or "".
func firstOfType(rrs []ResourceRecord, t uint16) string {
	for _, rr := range rrs {
		if rr.Type == t {
			return rr.Data
		}
	}
	return ""
}
