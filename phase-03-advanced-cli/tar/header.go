// Package main implements a from-scratch `tar` archiver speaking the POSIX
// USTAR format. This file owns the most important idea in the whole program:
// the on-disk 512-byte HEADER BLOCK. Everything tar does — create, list,
// extract — is built on reading and writing these fixed-size records.
//
// 🐍➡️🐹 Python analogy: think of a header as a `struct.pack`/`struct.unpack`
// of a C struct with a fixed byte layout. In Python you'd reach for the
// `struct` module; in Go we slice a `[blockSize]byte` array at fixed offsets.
package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// blockSize is the atom of the tar format. EVERYTHING is a multiple of 512
// bytes: every header is exactly one block, and file data is padded up to the
// next whole block. This fixed grid is what lets a reader stream an archive
// without ever seeking — read 512 bytes, you know what you're holding.
const blockSize = 512

// The USTAR header is a packed C struct. Go has no built-in "struct at byte
// offsets" the way C does, so we name every field's [start, end) byte range as
// explicit constants. Writing them out like this is the clearest possible map
// of the spec onto code — each line is one row of the header diagram.
//
// Offsets are half-open: [off, off+len). All numeric fields are stored as
// ASCII OCTAL digits (not binary!), a 1970s portability choice so the format
// is the same on every CPU regardless of byte order.
const (
	offName     = 0   // 100 bytes: file path (up to 100 chars)
	offMode     = 100 //   8 bytes: permission bits, octal
	offUID      = 108 //   8 bytes: owner user id, octal
	offGID      = 116 //   8 bytes: owner group id, octal
	offSize     = 124 //  12 bytes: file size in bytes, octal
	offMTime    = 136 //  12 bytes: modification time (Unix seconds), octal
	offChecksum = 148 //   8 bytes: header checksum, octal (see computeChecksum)
	offType     = 156 //   1 byte:  type flag ('0' file, '5' dir, …)
	offLinkname = 157 // 100 bytes: target path for links
	offMagic    = 257 //   6 bytes: "ustar\0" — identifies the USTAR variant
	offVersion  = 263 //   2 bytes: "00"
	offUname    = 265 //  32 bytes: owner user name
	offGname    = 297 //  32 bytes: owner group name
	offDevMajor = 329 //   8 bytes: device major (for device files)
	offDevMinor = 337 //   8 bytes: device minor
	offPrefix   = 345 // 155 bytes: path prefix — extends `name` past 100 chars
)

// Type flags. A single ASCII byte at offset 156 tells the reader what kind of
// entry this is. We support the two that cover real-world directory trees.
const (
	typeRegular = '0' // a normal file (older tars used '\0'; we accept both)
	typeDir     = '5' // a directory (its data section is empty)
)

// ustarMagic is the literal that marks a header as POSIX USTAR. Note the
// trailing NUL: the field is "ustar\0", which is how we tell USTAR apart from
// the older, pre-standard "v7" tar (which left this area blank).
const ustarMagic = "ustar\x00"

// header is our friendly, decoded view of one 512-byte block. The reader
// produces these from raw bytes; the writer consumes them to produce raw
// bytes. Keeping this struct separate from the wire format means the rest of
// the program never pokes at byte offsets directly.
type header struct {
	Name     string
	Mode     int64
	UID      int64
	GID      int64
	Size     int64
	ModTime  int64 // Unix seconds
	TypeFlag byte
	Linkname string
}

// IsDir reports whether this header describes a directory.
func (h *header) IsDir() bool { return h.TypeFlag == typeDir }

// ---------------------------------------------------------------------------
// ENCODE: header struct  ->  raw 512-byte block
// ---------------------------------------------------------------------------

// encode renders the header into a single 512-byte block ready to write to the
// archive. The checksum is computed last, over the fully-populated block, so it
// must run after every other field is in place.
func (h *header) encode() ([blockSize]byte, error) {
	var blk [blockSize]byte

	// USTAR splits long paths across `prefix` (155 bytes) + `name` (100 bytes).
	// splitPath decides where the boundary goes; together they reconstruct the
	// full path on read.
	prefix, name, err := splitPath(h.Name)
	if err != nil {
		return blk, err
	}

	// writeString copies an ASCII string into a field, left-aligned and
	// NUL-padded — the format's convention for text fields.
	writeString(blk[offName:offName+100], name)
	writeString(blk[offLinkname:offLinkname+100], h.Linkname)
	writeString(blk[offPrefix:offPrefix+155], prefix)

	// Numeric fields are ASCII octal, right-aligned, zero-padded, with a
	// trailing NUL (or space). writeOctal handles that layout.
	writeOctal(blk[offMode:offMode+8], h.Mode)
	writeOctal(blk[offUID:offUID+8], h.UID)
	writeOctal(blk[offGID:offGID+8], h.GID)
	writeOctal(blk[offSize:offSize+12], h.Size)
	writeOctal(blk[offMTime:offMTime+12], h.ModTime)

	blk[offType] = h.TypeFlag

	// Stamp the USTAR identity so other tools recognise the format.
	copy(blk[offMagic:offMagic+6], ustarMagic)
	copy(blk[offVersion:offVersion+2], "00")

	// The checksum must be calculated with the checksum field itself treated as
	// 8 spaces (see computeChecksum), then written back in. This ordering is a
	// classic gotcha — get it wrong and every other tar rejects your archive.
	writeChecksum(&blk)

	return blk, nil
}

// writeChecksum fills the checksum field per the spec: compute the sum with the
// field blanked to spaces, then store it as six octal digits, a NUL, and a
// space.
func writeChecksum(blk *[blockSize]byte) {
	sum := computeChecksum(blk)
	// Format: 6 octal digits, then '\0', then ' '. Real tars are lenient on
	// the last two bytes; this layout is the most widely accepted.
	field := blk[offChecksum : offChecksum+8]
	s := fmt.Sprintf("%06o", sum)
	copy(field, s)
	field[6] = 0
	field[7] = ' '
}

// computeChecksum returns the simple unsigned sum of all 512 bytes, BUT with
// the 8 checksum bytes themselves counted as ASCII spaces (0x20). Why spaces?
// Because the checksum can't include itself — so the spec defines a fixed
// placeholder value for those 8 bytes while summing. This is the single most
// important integrity check in the format.
func computeChecksum(blk *[blockSize]byte) int64 {
	var sum int64
	for i := 0; i < blockSize; i++ {
		if i >= offChecksum && i < offChecksum+8 {
			sum += ' ' // treat checksum field as spaces
		} else {
			sum += int64(blk[i])
		}
	}
	return sum
}

// ---------------------------------------------------------------------------
// DECODE: raw 512-byte block  ->  header struct
// ---------------------------------------------------------------------------

// errZeroBlock signals an all-zero block. Two of these in a row mark the end of
// the archive, so the reader needs to distinguish "empty terminator" from a
// real header rather than treating it as a parse error.
var errZeroBlock = errors.New("zero block")

// decodeHeader parses a raw 512-byte block into a header. It verifies the
// checksum so corruption or non-tar data is caught immediately.
func decodeHeader(blk *[blockSize]byte) (*header, error) {
	if isZeroBlock(blk) {
		return nil, errZeroBlock
	}

	// Integrity first: recompute the checksum and compare it to the stored one.
	// A mismatch means this isn't a valid header (corruption, or not a tar).
	stored, err := parseOctal(blk[offChecksum : offChecksum+8])
	if err != nil {
		return nil, fmt.Errorf("bad checksum field: %w", err)
	}
	if got := computeChecksum(blk); got != stored {
		return nil, fmt.Errorf("checksum mismatch: header says %d, computed %d", stored, got)
	}

	h := &header{}

	name := parseString(blk[offName : offName+100])
	prefix := parseString(blk[offPrefix : offPrefix+155])
	// Rejoin the split path: prefix + "/" + name (only if prefix is present).
	if prefix != "" {
		h.Name = prefix + "/" + name
	} else {
		h.Name = name
	}

	h.Linkname = parseString(blk[offLinkname : offLinkname+100])

	if h.Mode, err = parseOctal(blk[offMode : offMode+8]); err != nil {
		return nil, fmt.Errorf("bad mode: %w", err)
	}
	if h.UID, err = parseOctal(blk[offUID : offUID+8]); err != nil {
		return nil, fmt.Errorf("bad uid: %w", err)
	}
	if h.GID, err = parseOctal(blk[offGID : offGID+8]); err != nil {
		return nil, fmt.Errorf("bad gid: %w", err)
	}
	if h.Size, err = parseOctal(blk[offSize : offSize+12]); err != nil {
		return nil, fmt.Errorf("bad size: %w", err)
	}
	if h.ModTime, err = parseOctal(blk[offMTime : offMTime+12]); err != nil {
		return nil, fmt.Errorf("bad mtime: %w", err)
	}

	h.TypeFlag = blk[offType]
	// Older tars use NUL to mean "regular file"; normalise it to '0' so the
	// rest of the program only has to handle one spelling.
	if h.TypeFlag == 0 {
		h.TypeFlag = typeRegular
	}

	return h, nil
}

// ---------------------------------------------------------------------------
// Field codecs (the low-level byte<->value helpers)
// ---------------------------------------------------------------------------

// writeString left-aligns s into dst and NUL-pads the rest. If s is longer than
// the field it is silently truncated to fit — callers (splitPath) ensure paths
// fit before we get here.
func writeString(dst []byte, s string) {
	for i := range dst {
		dst[i] = 0
	}
	copy(dst, s)
}

// parseString reads a NUL-terminated (or full-width) ASCII string out of a
// field, dropping the trailing NUL padding.
func parseString(field []byte) string {
	if i := indexByte(field, 0); i >= 0 {
		field = field[:i]
	}
	return string(field)
}

// writeOctal stores v as zero-padded ASCII octal, right-aligned, with a single
// trailing NUL in the last byte (the format's convention for numeric fields).
// Example: mode 0644 in an 8-byte field becomes "0000644\0".
func writeOctal(dst []byte, v int64) {
	// One byte is reserved for the trailing NUL terminator, so we have
	// len(dst)-1 digits to play with.
	digits := len(dst) - 1
	s := strconv.FormatInt(v, 8)
	if len(s) > digits {
		// Value too big for the field; keep the low-order digits. In practice
		// our sizes/modes always fit, so this is just defensive.
		s = s[len(s)-digits:]
	}
	// Zero-pad on the left to fill the field.
	pad := strings.Repeat("0", digits-len(s))
	copy(dst, pad+s)
	dst[len(dst)-1] = 0
}

// parseOctal reads an ASCII octal numeric field. Real archives pad these with
// leading/trailing spaces or NULs, so we trim that noise before parsing. An
// all-blank field means zero.
func parseOctal(field []byte) (int64, error) {
	// Trim the surrounding NULs and spaces that different tars use as padding.
	trimmed := strings.Trim(string(field), " \x00")
	if trimmed == "" {
		return 0, nil
	}
	return strconv.ParseInt(trimmed, 8, 64)
}

// isZeroBlock reports whether every byte in the block is zero — the building
// block of the two-zero-block end-of-archive marker.
func isZeroBlock(blk *[blockSize]byte) bool {
	for _, b := range blk {
		if b != 0 {
			return false
		}
	}
	return true
}

// indexByte is a tiny helper (avoids importing bytes just for one call) that
// returns the first index of c in b, or -1.
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// splitPath divides a path into the USTAR (prefix, name) pair so paths longer
// than 100 bytes still fit. The rule from the spec: `name` holds up to 100
// bytes, `prefix` up to 155, and the reader rejoins them as prefix+"/"+name.
// We split at a "/" boundary so we never cut a path component in half.
func splitPath(path string) (prefix, name string, err error) {
	if len(path) <= 100 {
		return "", path, nil
	}
	// Find a "/" such that the tail (name) is <= 100 bytes and the head
	// (prefix) is <= 155 bytes. Walk from the rightmost slash that keeps name
	// within budget.
	if len(path) > 100+1+155 {
		return "", "", fmt.Errorf("path too long for ustar (max 256 bytes): %q", path)
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] != '/' {
			continue
		}
		head, tail := path[:i], path[i+1:]
		if len(tail) <= 100 && len(head) <= 155 {
			return head, tail, nil
		}
	}
	return "", "", fmt.Errorf("cannot split path to fit ustar fields: %q", path)
}
