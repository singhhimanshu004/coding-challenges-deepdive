// conn.go parses and serves the memcached TEXT protocol on a single connection.
//
// 📦 The text protocol has two shapes of message:
//
//  1. A single COMMAND LINE terminated by CRLF (\r\n), e.g.:
//     get foo\r\n
//     delete foo\r\n
//     incr foo 2\r\n
//
//  2. A STORAGE command line + a DATA BLOCK. The command line announces the
//     exact byte length of the data that follows; the data block is that many
//     raw bytes plus a trailing CRLF (which is NOT counted in the length):
//     set foo 0 0 3\r\n     <- key, flags, exptime, bytes=3
//     bar\r\n               <- exactly 3 bytes "bar", then CRLF
//
// 🐍 For a Python dev: this is "read a line, split on spaces, dispatch" — like a
// tiny REPL over a socket. The only twist is that storage commands then read an
// exact number of raw bytes (length-prefixed framing), so we never guess where
// the value ends.
package main

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
)

// crlf is the protocol line terminator. Every line and every data block ends
// with it; getting this wrong is the #1 beginner bug (same lesson as in curl).
const crlf = "\r\n"

// maxValueBytes guards against a client claiming a huge data block. memcached's
// real default item cap is 1 MiB; we use the same number.
const maxValueBytes = 1024 * 1024

// serveConn reads commands from r and writes replies to w until the client
// disconnects (io.EOF) or a fatal I/O error occurs.
func serveConn(r io.Reader, w io.Writer, store *Store, verbose bool, logf func(string, ...any)) error {
	// 🐍 bufio.Reader/Writer are buffered wrappers — like Python's io.BufferedReader.
	// ReadString lets us read a line at a time; the writer batches small writes.
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)

	for {
		line, err := readLine(br)
		if err != nil {
			return err // io.EOF on a clean client disconnect
		}
		if line == "" {
			continue // ignore blank lines between commands
		}
		if verbose {
			logf("<- %q", line)
		}

		fields := strings.Fields(line)
		cmd := fields[0]

		// dispatch returns false to signal "client asked us to quit".
		keepGoing := dispatch(br, bw, store, cmd, fields[1:])
		if ferr := bw.Flush(); ferr != nil {
			return ferr
		}
		if !keepGoing {
			return nil
		}
	}
}

// dispatch routes one command to its handler. args excludes the command word.
func dispatch(br *bufio.Reader, bw *bufio.Writer, store *Store, cmd string, args []string) bool {
	switch cmd {
	case "set", "add", "replace", "append", "prepend", "cas":
		handleStorage(br, bw, store, cmd, args)
	case "get", "gets":
		handleGet(bw, store, cmd, args)
	case "delete":
		handleDelete(bw, store, args)
	case "incr", "decr":
		handleIncrDecr(bw, store, cmd, args)
	case "flush_all":
		handleFlushAll(bw, store, args)
	case "version":
		writeLine(bw, "VERSION cc-memcached/1.0")
	case "quit":
		return false
	default:
		writeLine(bw, "ERROR") // unknown command, per the protocol
	}
	return true
}

// --- Storage commands: set / add / replace / append / prepend / cas ---
//
// Line shapes:
//
//	<set|add|replace|append|prepend> <key> <flags> <exptime> <bytes> [noreply]
//	cas <key> <flags> <exptime> <bytes> <cas unique> [noreply]
//
// then a data block of <bytes> raw bytes + CRLF.
func handleStorage(br *bufio.Reader, bw *bufio.Writer, store *Store, cmd string, args []string) {
	isCAS := cmd == "cas"
	minArgs := 4
	if isCAS {
		minArgs = 5
	}
	if len(args) < minArgs {
		// We still must consume nothing extra; just report the framing error.
		writeLine(bw, "ERROR")
		return
	}

	key := args[0]
	flags64, e1 := strconv.ParseUint(args[1], 10, 32)
	exptime, e2 := strconv.ParseInt(args[2], 10, 64)
	nbytes, e3 := strconv.Atoi(args[3])

	var casUnique uint64
	var e4 error
	noreplyIdx := 4
	if isCAS {
		casUnique, e4 = strconv.ParseUint(args[4], 10, 64)
		noreplyIdx = 5
	}
	noreply := len(args) > noreplyIdx && args[noreplyIdx] == "noreply"

	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || nbytes < 0 {
		// The client still sent a data block; try to drain it so we stay in sync.
		if e3 == nil && nbytes >= 0 && nbytes <= maxValueBytes {
			_, _ = readDataBlock(br, nbytes)
		}
		writeLineNoreply(bw, "CLIENT_ERROR bad command line format", noreply)
		return
	}
	if nbytes > maxValueBytes {
		writeLineNoreply(bw, "SERVER_ERROR object too large for cache", noreply)
		return
	}

	data, derr := readDataBlock(br, nbytes)
	if derr != nil {
		writeLineNoreply(bw, "CLIENT_ERROR bad data chunk", noreply)
		return
	}

	flags := uint32(flags64)
	var res StoreResult
	switch cmd {
	case "set":
		res = store.Set(key, flags, exptime, data)
	case "add":
		res = store.Add(key, flags, exptime, data)
	case "replace":
		res = store.Replace(key, flags, exptime, data)
	case "append":
		res = store.Append(key, data)
	case "prepend":
		res = store.Prepend(key, data)
	case "cas":
		res = store.CAS(key, flags, exptime, casUnique, data)
	}
	writeLineNoreply(bw, storeResultLine(res), noreply)
}

// storeResultLine maps a StoreResult to its wire reply.
func storeResultLine(r StoreResult) string {
	switch r {
	case Stored:
		return "STORED"
	case NotStored:
		return "NOT_STORED"
	case Exists:
		return "EXISTS"
	default:
		return "NOT_FOUND"
	}
}

// --- Retrieval: get / gets ---
//
// Line: get <key>+   (one or more keys)
// Reply per found key:
//
//	VALUE <key> <flags> <bytes>[ <cas>]\r\n
//	<data>\r\n
//
// then a single END\r\n. gets includes the CAS token; get does not.
func handleGet(bw *bufio.Writer, store *Store, cmd string, keys []string) {
	if len(keys) == 0 {
		writeLine(bw, "ERROR")
		return
	}
	withCAS := cmd == "gets"
	for _, r := range store.Get(keys) {
		header := "VALUE " + r.Key + " " + strconv.FormatUint(uint64(r.Flags), 10) +
			" " + strconv.Itoa(len(r.Value))
		if withCAS {
			header += " " + strconv.FormatUint(r.CAS, 10)
		}
		writeLine(bw, header)
		bw.Write(r.Value) // raw bytes, then the framing CRLF
		bw.WriteString(crlf)
	}
	writeLine(bw, "END")
}

// --- delete <key> [noreply] ---
func handleDelete(bw *bufio.Writer, store *Store, args []string) {
	if len(args) < 1 {
		writeLine(bw, "ERROR")
		return
	}
	noreply := len(args) > 1 && args[len(args)-1] == "noreply"
	if store.Delete(args[0]) {
		writeLineNoreply(bw, "DELETED", noreply)
	} else {
		writeLineNoreply(bw, "NOT_FOUND", noreply)
	}
}

// --- incr/decr <key> <delta> [noreply] ---
func handleIncrDecr(bw *bufio.Writer, store *Store, cmd string, args []string) {
	if len(args) < 2 {
		writeLine(bw, "ERROR")
		return
	}
	noreply := len(args) > 2 && args[len(args)-1] == "noreply"
	delta, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		writeLineNoreply(bw, "CLIENT_ERROR invalid numeric delta argument", noreply)
		return
	}
	newVal, found, derr := store.IncrDecr(args[0], delta, cmd == "decr")
	switch {
	case errors.Is(derr, errNotNumeric):
		writeLineNoreply(bw, "CLIENT_ERROR "+errNotNumeric.Error(), noreply)
	case !found:
		writeLineNoreply(bw, "NOT_FOUND", noreply)
	default:
		writeLineNoreply(bw, strconv.FormatUint(newVal, 10), noreply)
	}
}

// --- flush_all [delay] [noreply] ---
// We honour the immediate form (delay is parsed but treated as 0 for simplicity).
func handleFlushAll(bw *bufio.Writer, store *Store, args []string) {
	noreply := len(args) > 0 && args[len(args)-1] == "noreply"
	store.FlushAll()
	writeLineNoreply(bw, "OK", noreply)
}

// --- low-level framing helpers ---

// readLine reads up to and including the next '\n', then strips the trailing
// CRLF (or lone LF). It returns io.EOF when the client has gone away.
func readLine(br *bufio.Reader) (string, error) {
	s, err := br.ReadString('\n')
	if err != nil {
		if err == io.EOF && s != "" {
			// A final line without a newline: still usable.
			return strings.TrimRight(s, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// readDataBlock reads exactly n bytes (the announced value length) and then
// consumes the trailing CRLF. io.ReadFull guarantees we get all n bytes or an
// error — no short reads, no guessing where the value ends.
func readDataBlock(br *bufio.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, err
	}
	// The protocol requires the value to be followed by CRLF. Verify it.
	cr, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	lf, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	if cr != '\r' || lf != '\n' {
		return nil, errors.New("bad data chunk: missing CRLF terminator")
	}
	return buf, nil
}

// writeLine writes s followed by CRLF.
func writeLine(bw *bufio.Writer, s string) {
	bw.WriteString(s)
	bw.WriteString(crlf)
}

// writeLineNoreply writes a reply unless the client asked for noreply.
func writeLineNoreply(bw *bufio.Writer, s string, noreply bool) {
	if noreply {
		return
	}
	writeLine(bw, s)
}
