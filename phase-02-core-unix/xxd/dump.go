// dump.go — the forward engine: turn a stream of bytes into the classic
// xxd hex dump. The output of `dump` is byte-for-byte compatible with the
// system xxd for the flags this tool supports.
package main

import (
	"bufio"
	"fmt"
	"io"
)

// config is the fully-parsed request describing how to format (or reverse) a
// dump. Zero values are never used directly — parseArgs seeds the defaults.
type config struct {
	cols      int   // -c : bytes shown per output line
	group     int   // -g : bytes per space-separated group
	seek      int64 // -s : bytes to skip before dumping
	length    int64 // -l : maximum bytes to dump
	hasLength bool  // whether -l was supplied
	reverse   bool  // -r : reverse mode (parse a dump back to bytes)
}

// dump reads bytes from r and writes a hex dump to w following cfg.
//
// The anatomy of one output line (default -c 16 -g 2):
//
//	00000000: 6865 6c6c 6f20 776f 726c 640a 7365 636f  hello world.seco
//	└─offset─┘ └────────────── hex columns ──────────┘  └─ ASCII gutter ┘
//
// Each line is: an 8-digit hex offset, a colon, the hex byte columns grouped
// `group` bytes at a time, two separator spaces, then the printable-ASCII
// rendering (non-printables shown as '.').
func dump(r io.Reader, w io.Writer, cfg config) error {
	// bufio wraps the raw reader/writer so we are not doing a syscall per byte.
	// Go idiom: defer the Flush so buffered output is always written on return,
	// like a `finally` block in Python/Java.
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	// Honour -s by discarding `seek` bytes up front. io.CopyN to io.Discard is
	// the idiomatic "skip N bytes" — it works on a pipe where Seek is illegal,
	// keeping us binary- and stdin-safe.
	if cfg.seek > 0 {
		if _, err := io.CopyN(io.Discard, br, cfg.seek); err != nil {
			if err == io.EOF {
				return nil // sought past end of input: nothing to dump
			}
			return err
		}
	}

	// line holds one row of raw bytes; reusing the slice avoids re-allocating
	// it on every iteration.
	line := make([]byte, cfg.cols)
	offset := cfg.seek          // the address printed in the offset column
	var emitted int64           // bytes dumped so far (for -l accounting)

	for {
		// Clamp this line's read size so we never exceed the -l budget.
		want := cfg.cols
		if cfg.hasLength {
			remaining := cfg.length - emitted
			if remaining <= 0 {
				break
			}
			if int64(want) > remaining {
				want = int(remaining)
			}
		}

		// io.ReadFull keeps reading until the buffer is full or input ends; a
		// short final line surfaces as ErrUnexpectedEOF with n > 0.
		n, err := io.ReadFull(br, line[:want])
		if n > 0 {
			writeLine(bw, offset, line[:n], cfg)
			offset += int64(n)
			emitted += int64(n)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// writeLine formats a single dump line for the bytes in `data` at `offset`.
func writeLine(bw *bufio.Writer, offset int64, data []byte, cfg config) {
	// Offset column: 8 hex digits and a colon, e.g. "0000001f:".
	fmt.Fprintf(bw, "%08x:", offset)

	// Hex columns. We walk all `cols` slots (not just the bytes present) so a
	// short final line is padded to a constant width and the ASCII gutter stays
	// aligned. A single space precedes each group; missing bytes print as two
	// spaces instead of two hex digits.
	for i := 0; i < cfg.cols; i++ {
		if i%cfg.group == 0 {
			bw.WriteByte(' ')
		}
		if i < len(data) {
			fmt.Fprintf(bw, "%02x", data[i])
		} else {
			bw.WriteString("  ")
		}
	}

	// Two spaces separate the hex columns from the ASCII gutter.
	bw.WriteString("  ")

	// ASCII gutter: printable bytes verbatim, everything else as '.'.
	for _, b := range data {
		if b >= 0x20 && b <= 0x7e {
			bw.WriteByte(b)
		} else {
			bw.WriteByte('.')
		}
	}
	bw.WriteByte('\n')
}
