package huffman

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"huffman/internal/bitio"
)

// magic identifies our container format and version. If we change the header
// layout later, bump this so old files are rejected instead of misread.
var magic = []byte("HUF1")

// ErrBadFormat is returned when the input is not a valid HUF1 stream.
var ErrBadFormat = errors.New("huffman: not a valid HUF1 file")

// Header / file layout (all multi-byte integers are big-endian):
//
//	+---------+-------------+-------------+----------------------------+--------------+
//	| "HUF1"  | totalSymbols| numDistinct | table: [sym][uvarint freq] | packed bits  |
//	| 4 bytes |  uint64     |  uint16     |   numDistinct entries      | body         |
//	+---------+-------------+-------------+----------------------------+--------------+
//
// DESIGN DECISION — store the frequency table, not the codes.
// We persist the (symbol, frequency) pairs and rebuild the identical tree on
// decode. The alternative is canonical Huffman: store only code LENGTHS and
// reconstruct codes by a fixed rule, which is more compact (1 byte/symbol vs a
// varint). We chose the frequency table because it is the most direct, easiest
// to verify teaching artifact and the overhead (a few hundred bytes at most) is
// negligible for non-trivial inputs. The trade-off is documented in the README.
//
// totalSymbols lets the decoder stop after exactly the right number of symbols,
// which is what makes the zero-padding of the final byte safe.

// Compress encodes data into the HUF1 container format.
func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	freqs := CountFrequencies(data)

	// --- Header ---
	buf.Write(magic)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], uint64(len(data)))
	buf.Write(u64[:])
	var u16 [2]byte
	binary.BigEndian.PutUint16(u16[:], uint16(len(freqs)))
	buf.Write(u16[:])
	for sym, f := range freqs {
		buf.WriteByte(sym)
		var tmp [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(tmp[:], f)
		buf.Write(tmp[:n])
	}

	// --- Body ---
	if len(data) > 0 {
		root := buildTree(freqs)
		codes := buildCodes(root)
		bw := bitio.NewWriter(&buf)
		for _, b := range data {
			if err := bw.WriteBits(codes[b]); err != nil {
				return nil, err
			}
		}
		if err := bw.Flush(); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Decompress reverses Compress, recovering the original bytes exactly.
func Decompress(data []byte) ([]byte, error) {
	r := bufio.NewReader(bytes.NewReader(data))

	// --- Header ---
	gotMagic := make([]byte, len(magic))
	if _, err := io.ReadFull(r, gotMagic); err != nil {
		return nil, ErrBadFormat
	}
	if !bytes.Equal(gotMagic, magic) {
		return nil, ErrBadFormat
	}
	var u64 [8]byte
	if _, err := io.ReadFull(r, u64[:]); err != nil {
		return nil, ErrBadFormat
	}
	total := binary.BigEndian.Uint64(u64[:])
	var u16 [2]byte
	if _, err := io.ReadFull(r, u16[:]); err != nil {
		return nil, ErrBadFormat
	}
	numDistinct := int(binary.BigEndian.Uint16(u16[:]))

	freqs := make(map[byte]uint64, numDistinct)
	for i := 0; i < numDistinct; i++ {
		sym, err := r.ReadByte()
		if err != nil {
			return nil, ErrBadFormat
		}
		f, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, ErrBadFormat
		}
		freqs[sym] = f
	}

	// Empty original file: nothing to decode.
	if total == 0 {
		return []byte{}, nil
	}

	root := buildTree(freqs)
	out := make([]byte, 0, total)

	// Single-symbol input has no internal nodes to walk; the body carries no
	// information, so just replay the lone symbol `total` times.
	if root.leaf {
		for i := uint64(0); i < total; i++ {
			out = append(out, root.symbol)
		}
		return out, nil
	}

	// --- Body --- walk the tree one bit at a time until we have `total` bytes.
	br := bitio.NewReader(r)
	n := root
	for uint64(len(out)) < total {
		bit, err := br.ReadBit()
		if err != nil {
			return nil, fmt.Errorf("huffman: truncated body: %w", err)
		}
		if bit == 0 {
			n = n.left
		} else {
			n = n.right
		}
		if n.leaf {
			out = append(out, n.symbol)
			n = root
		}
	}
	return out, nil
}

// Ratio reports the compression ratio as compressed/original (smaller is
// better). It returns 0 when the original is empty to avoid dividing by zero.
func Ratio(originalSize, compressedSize int) float64 {
	if originalSize == 0 {
		return 0
	}
	return float64(compressedSize) / float64(originalSize)
}
