package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

// subscription records one SUB issued by a client.
//
//	subject : the (possibly wildcarded) pattern the client is interested in
//	queue   : queue-group name, or "" for a plain subscription
//	sid     : the client-chosen subscription id, echoed back on every MSG
//	max     : auto-unsubscribe after this many messages (0 = unlimited)
//	count   : how many messages have been delivered so far
type subscription struct {
	sid     string
	subject string
	queue   string
	client  *client
	max     int
	count   int
}

// Server is the broker. It owns the listener and the subscription registry.
//
// The registry is a map keyed by *client, each value a map of sid -> sub. This
// nested shape makes the two hot operations cheap:
//   - removing a disconnected client: delete one top-level key.
//   - matching a publish: iterate every subscription and test the subject.
//
// A single sync.Mutex guards the registry because publishes, subscribes and
// disconnects all race on it. 🐍 Python note: sync.Mutex is like
// threading.Lock(); the `defer s.mu.Unlock()` idiom is the equivalent of a
// `with lock:` block — the unlock runs when the function returns.
type Server struct {
	addr    string
	verbose bool

	mu     sync.Mutex
	subs   map[*client]map[string]*subscription
	nextID uint64
	rr     uint64 // round-robin counter for queue-group load balancing

	ln net.Listener
}

// NewServer builds a broker that will listen on addr.
func NewServer(addr string, verbose bool) *Server {
	return &Server{
		addr:    addr,
		verbose: verbose,
		subs:    make(map[*client]map[string]*subscription),
	}
}

// Listen opens the TCP listener. Split from Serve so tests can grab the chosen
// port (when listening on ":0") via Addr() before the accept loop starts.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Addr returns the actual listening address, including the OS-assigned port
// when the server was started on ":0".
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.addr
	}
	return s.ln.Addr().String()
}

// Serve runs the accept loop, spawning one goroutine per accepted connection.
func (s *Server) Serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleClient(conn)
	}
}

// Close stops the listener, ending the accept loop.
func (s *Server) Close() error {
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// handleClient is the per-connection goroutine. It greets the client with INFO,
// starts the writer goroutine, then reads and dispatches commands line-by-line
// until the connection drops.
func (s *Server) handleClient(conn net.Conn) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	c := &client{
		id:      id,
		conn:    conn,
		srv:     s,
		out:     make(chan []byte, 256),
		quit:    make(chan struct{}),
		verbose: s.verbose,
	}
	go c.writeLoop()
	defer s.removeClient(c)

	if s.verbose {
		log.Printf("client %d connected from %s", id, conn.RemoteAddr())
	}

	// The protocol greeting: a real client waits for INFO before doing anything.
	c.enqueue([]byte(`INFO {"server_id":"natsbroker","version":"0.1.0","max_payload":1048576}` + "\r\n"))

	// 🐍 Python note: bufio.Reader is a buffered wrapper, like wrapping a socket
	// file object. ReadString('\n') reads up to and including the newline — the
	// NATS protocol is line-based, so one control line == one read.
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return // EOF or read error: client gone.
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if err := s.processCommand(c, r, line); err != nil {
			c.err(err.Error())
		}
	}
}

// processCommand parses one control line and dispatches on its verb. The verb
// is case-insensitive (NATS treats PUB and pub identically).
//
// PUB is special: its control line is followed by a payload of exactly the
// announced number of bytes, so it needs the bufio.Reader to read that body.
func (s *Server) processCommand(c *client, r *bufio.Reader, line string) error {
	fields := strings.Fields(line)
	switch strings.ToUpper(fields[0]) {
	case "PING":
		c.enqueue([]byte("PONG\r\n"))
		return nil
	case "PONG":
		return nil // client's reply to our PING; nothing to do.
	case "CONNECT":
		// Honour the verbose flag from the CONNECT options if present.
		if strings.Contains(line, `"verbose":true`) {
			c.verbose = true
		} else if strings.Contains(line, `"verbose":false`) {
			c.verbose = false
		}
		c.ok()
		return nil
	case "SUB":
		return s.handleSub(c, fields)
	case "UNSUB":
		return s.handleUnsub(c, fields)
	case "PUB":
		return s.handlePub(c, r, fields)
	default:
		return fmt.Errorf("Unknown Protocol Operation")
	}
}

// handleSub registers a subscription. Syntax:
//
//	SUB <subject> <sid>
//	SUB <subject> <queue> <sid>
func (s *Server) handleSub(c *client, f []string) error {
	var subject, queue, sid string
	switch len(f) {
	case 3:
		subject, sid = f[1], f[2]
	case 4:
		subject, queue, sid = f[1], f[2], f[3]
	default:
		return fmt.Errorf("Invalid Subscribe arguments")
	}

	sub := &subscription{sid: sid, subject: subject, queue: queue, client: c}
	s.mu.Lock()
	if s.subs[c] == nil {
		s.subs[c] = make(map[string]*subscription)
	}
	s.subs[c][sid] = sub
	s.mu.Unlock()

	c.ok()
	return nil
}

// handleUnsub removes a subscription, or arms it for auto-removal after N more
// messages. Syntax:
//
//	UNSUB <sid>
//	UNSUB <sid> <max_msgs>
func (s *Server) handleUnsub(c *client, f []string) error {
	if len(f) < 2 || len(f) > 3 {
		return fmt.Errorf("Invalid Unsubscribe arguments")
	}
	sid := f[1]

	max := 0
	if len(f) == 3 {
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 0 {
			return fmt.Errorf("Invalid Number of Messages")
		}
		max = n
	}

	s.mu.Lock()
	if bySid := s.subs[c]; bySid != nil {
		if sub, ok := bySid[sid]; ok {
			if max > 0 && sub.count < max {
				sub.max = max // remove later, once `max` total messages delivered.
			} else {
				delete(bySid, sid) // immediate unsubscribe.
			}
		}
	}
	s.mu.Unlock()

	c.ok()
	return nil
}

// handlePub publishes a message. The control line announces the payload size;
// the payload (exactly that many bytes) and a trailing CRLF follow. Syntax:
//
//	PUB <subject> <#bytes>\r\n<payload>\r\n
//	PUB <subject> <reply> <#bytes>\r\n<payload>\r\n
func (s *Server) handlePub(c *client, r *bufio.Reader, f []string) error {
	var subject, reply, sizeStr string
	switch len(f) {
	case 3:
		subject, sizeStr = f[1], f[2]
	case 4:
		subject, reply, sizeStr = f[1], f[2], f[3]
	default:
		return fmt.Errorf("Invalid Publish arguments")
	}

	n, err := strconv.Atoi(sizeStr)
	if err != nil || n < 0 {
		return fmt.Errorf("Invalid Number of Bytes")
	}

	// io.ReadFull reads EXACTLY n bytes — the size header is how we frame the
	// payload, since the payload itself may contain newlines.
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("Bad Payload")
	}
	// Consume the trailing CRLF that terminates the payload.
	if _, err := r.Discard(2); err != nil {
		return fmt.Errorf("Missing Payload Terminator")
	}

	s.deliver(subject, reply, payload)
	c.ok()
	return nil
}

// deliver is the routing core. It finds every subscription whose pattern
// matches the published subject and fans the message out, honouring queue-group
// semantics: plain subscribers each get a copy, while subscribers sharing a
// queue-group name get the message delivered to exactly ONE of them.
func (s *Server) deliver(subject, reply string, payload []byte) {
	s.mu.Lock()

	var recipients []*subscription
	queues := make(map[string][]*subscription)

	for _, bySid := range s.subs {
		for _, sub := range bySid {
			if !matchSubject(sub.subject, subject) {
				continue
			}
			if sub.queue == "" {
				recipients = append(recipients, sub) // every plain sub gets it.
			} else {
				queues[sub.queue] = append(queues[sub.queue], sub)
			}
		}
	}

	// For each queue group, pick exactly one member (round-robin load balance).
	for _, members := range queues {
		chosen := members[s.rr%uint64(len(members))]
		s.rr++
		recipients = append(recipients, chosen)
	}

	// Build frames while still holding the lock so we can safely bump per-sub
	// counters and auto-remove subscriptions that hit their UNSUB max.
	type send struct {
		c     *client
		frame []byte
	}
	var sends []send
	for _, sub := range recipients {
		sends = append(sends, send{sub.client, buildMsg(subject, sub.sid, reply, payload)})
		sub.count++
		if sub.max > 0 && sub.count >= sub.max {
			delete(s.subs[sub.client], sub.sid)
		}
	}
	s.mu.Unlock()

	// Enqueue outside the lock: enqueue can block on a full channel, and we must
	// never hold the registry mutex while blocked on a slow consumer.
	for _, sd := range sends {
		sd.c.enqueue(sd.frame)
	}
}

// buildMsg renders an outbound MSG frame:
//
//	MSG <subject> <sid> <#bytes>\r\n<payload>\r\n
//	MSG <subject> <sid> <reply> <#bytes>\r\n<payload>\r\n
func buildMsg(subject, sid, reply string, payload []byte) []byte {
	var b strings.Builder
	if reply != "" {
		fmt.Fprintf(&b, "MSG %s %s %s %d\r\n", subject, sid, reply, len(payload))
	} else {
		fmt.Fprintf(&b, "MSG %s %s %d\r\n", subject, sid, len(payload))
	}
	b.Write(payload)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// removeClient drops every subscription owned by a departing client and tears
// the connection down. Called via defer when handleClient returns.
func (s *Server) removeClient(c *client) {
	s.mu.Lock()
	delete(s.subs, c)
	s.mu.Unlock()
	c.close()
	if s.verbose {
		log.Printf("client %d disconnected", c.id)
	}
}
