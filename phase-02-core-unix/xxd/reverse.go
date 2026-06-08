// reverse.go — the inverse engine: read a hex dump (as produced by `dump` or
// the system xxd) and reconstruct the original bytes. This is the `-r` mode.
//
// The round-trip `dump | reverse` must reproduce the original byte stream
// exactly, including embedded NULs and other non-printable bytes — that
// guarantee is what makes hex dumps a safe way to ship binary data through
// text-only channels.
package main

import (
	"bufio"
	"fmt"
	"io"
)

// reverse parses a hex dump from r and writes the decoded bytes to w.
//
// Each input line looks like:
//
//	00000000: 6865 6c6c 6f0a                           hello.
//	└ offset ┘└──────── hex columns ────────┘└ gutter ┘
//
// We read the offset (the hex before the colon) and the hex columns, and
// deliberately ignore the ASCII gutter — the bytes are fully described by the
// hex. The offset lets us reproduce gaps (e.g. from a dump made with -s) by
// padding with zero bytes, mirroring the system xxd.
func reverse(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Dump lines are short, but raise the cap so an unusually wide -c dump
	// still fits within one token.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	var written int64 // bytes emitted so far, used to honour offset gaps

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Split the optional "OFFSET:" prefix from the rest of the line. Go's
		// multiple-return assignment makes this a clean one-liner.
		offset, body, hasOffset := splitOffset(line)

		// If the line names an address beyond what we've written, pad the gap
		// with zero bytes so sparse dumps round-trip to the right layout.
		if hasOffset && offset > written {
			if err := padZeros(bw, offset-written); err != nil {
				return err
			}
			written = offset
		}

		bytes, err := parseHexColumns(body)
		if err != nil {
			return err
		}
		if _, err := bw.Write(bytes); err != nil {
			return err
		}
		written += int64(len(bytes))
	}
	return scanner.Err()
}

// splitOffset separates a leading "OFFSET:" address from the hex body. It
// returns the parsed offset, the remainder of the line, and whether an offset
// was actually present (a postscript-style line has none).
func splitOffset(line string) (offset int64, body string, ok bool) {
	for i := 0; i < len(line); i++ {
		if line[i] == ':' {
			n, err := parseHex(line[:i])
			if err != nil {
				return 0, line, false
			}
			return n, line[i+1:], true
		}
		// Bail out early if we hit something that cannot be part of a hex
		// address — then there is no offset prefix on this line.
		if !isHexDigit(line[i]) && line[i] != ' ' {
			break
		}
	}
	return 0, line, false
}

// parseHexColumns reads the hex byte columns from the body of a dump line,
// stopping at the ASCII gutter. The gutter is preceded by a run of two or more
// spaces, whereas the inter-group separators inside the hex area are always a
// single space — so a double space cleanly marks "hex ends here".
func parseHexColumns(body string) ([]byte, error) {
	var out []byte
	var nibbles []byte // pending hex digits not yet paired into a byte

	flush := func() error {
		// Pair up accumulated nibbles into bytes. xxd writes them two at a
		// time, but we tolerate a stray odd nibble just in case.
		for len(nibbles) >= 2 {
			hi := hexVal(nibbles[0])
			lo := hexVal(nibbles[1])
			out = append(out, byte(hi<<4|lo))
			nibbles = nibbles[2:]
		}
		if len(nibbles) == 1 {
			out = append(out, byte(hexVal(nibbles[0])))
			nibbles = nibbles[:0]
		}
		return nil
	}

	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case isHexDigit(c):
			nibbles = append(nibbles, c)
		case c == ' ':
			// A double (or longer) space means the ASCII gutter starts here:
			// stop reading hex. A single space is just a group separator.
			if i+1 < len(body) && body[i+1] == ' ' {
				if err := flush(); err != nil {
					return nil, err
				}
				return out, nil
			}
		default:
			// Any other character cannot appear in the hex columns; we've run
			// into the gutter (or garbage). Stop here.
			if err := flush(); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

// padZeros writes n zero bytes — used to reproduce gaps implied by offsets.
func padZeros(w io.Writer, n int64) error {
	zeros := make([]byte, 1024)
	for n > 0 {
		chunk := int64(len(zeros))
		if chunk > n {
			chunk = n
		}
		if _, err := w.Write(zeros[:chunk]); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}

// parseHex converts a (possibly space-padded) hex string to an int64.
func parseHex(s string) (int64, error) {
	var n int64
	seen := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			continue
		}
		if !isHexDigit(c) {
			return 0, fmt.Errorf("invalid hex offset %q", s)
		}
		n = n<<4 | int64(hexVal(c))
		seen = true
	}
	if !seen {
		return 0, fmt.Errorf("empty hex offset")
	}
	return n, nil
}

// isHexDigit reports whether c is a hexadecimal digit (either case).
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// hexVal maps a hex digit byte to its 0–15 value; callers guard with
// isHexDigit first.
func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default: // 'A'..'F'
		return int(c-'A') + 10
	}
}
