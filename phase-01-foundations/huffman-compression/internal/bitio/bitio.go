// Package bitio provides bit-level reading and writing on top of byte streams.
//
// Huffman codes are variable-length sequences of bits, but the operating system
// and io.Reader/io.Writer only deal in whole bytes (8 bits). The BitWriter and
// BitReader here are the glue: they accumulate bits in a one-byte buffer and
// flush a full byte to the underlying stream once eight bits are queued. This
// "pack bits into bytes" skill is reused later in tar, xxd, and binary network
// protocols (DNS, NTP), so it is worth implementing carefully and from scratch.
package bitio

import (
	"bufio"
	"io"
)

// BitWriter writes individual bits to an underlying io.Writer, most-significant
// bit first (big-endian bit order). Bits accumulate in `cur`; once 8 bits are
// buffered the byte is emitted. Always call Flush before discarding the writer
// so the final, partially-filled byte is written (zero-padded on the right).
type BitWriter struct {
	w     *bufio.Writer
	cur   byte // bit buffer; bits fill from MSB toward LSB
	nbits uint // number of valid bits currently in cur (0..7)
}

// NewWriter wraps w in a buffered BitWriter.
func NewWriter(w io.Writer) *BitWriter {
	return &BitWriter{w: bufio.NewWriter(w)}
}

// WriteBit writes a single bit. Any non-zero value is treated as a 1.
func (bw *BitWriter) WriteBit(bit uint) error {
	// Shift the new bit into the next free (less significant) slot. We fill
	// from the top so the first bit written ends up as the MSB of the byte —
	// this matches how a reader walks the tree left-to-right.
	bw.cur <<= 1
	if bit != 0 {
		bw.cur |= 1
	}
	bw.nbits++
	if bw.nbits == 8 {
		if err := bw.w.WriteByte(bw.cur); err != nil {
			return err
		}
		bw.cur = 0
		bw.nbits = 0
	}
	return nil
}

// WriteBits writes the string representation of a code, e.g. "0110". Only the
// characters '0' and '1' are meaningful; this keeps call sites readable since
// our code table maps symbols to bit-strings.
func (bw *BitWriter) WriteBits(code string) error {
	for i := 0; i < len(code); i++ {
		var bit uint
		if code[i] == '1' {
			bit = 1
		}
		if err := bw.WriteBit(bit); err != nil {
			return err
		}
	}
	return nil
}

// Flush emits any buffered partial byte (right-padded with zero bits) and then
// flushes the underlying buffered writer. The zero padding is harmless because
// the decoder is told exactly how many symbols to emit and stops before it can
// mistake padding for real data.
func (bw *BitWriter) Flush() error {
	if bw.nbits > 0 {
		bw.cur <<= (8 - bw.nbits) // left-justify the valid bits
		if err := bw.w.WriteByte(bw.cur); err != nil {
			return err
		}
		bw.cur = 0
		bw.nbits = 0
	}
	return bw.w.Flush()
}

// BitReader reads individual bits from an underlying io.Reader in the same
// MSB-first order the BitWriter produces.
type BitReader struct {
	r     *bufio.Reader
	cur   byte
	nbits uint // number of bits left to consume from cur (0..8)
}

// NewReader wraps r in a buffered BitReader.
func NewReader(r io.Reader) *BitReader {
	return &BitReader{r: bufio.NewReader(r)}
}

// ReadBit returns the next bit (0 or 1). It returns io.EOF only when the
// underlying stream is exhausted and no buffered bits remain.
func (br *BitReader) ReadBit() (uint, error) {
	if br.nbits == 0 {
		b, err := br.r.ReadByte()
		if err != nil {
			return 0, err
		}
		br.cur = b
		br.nbits = 8
	}
	br.nbits--
	// Pull the current MSB out, mirroring the writer's fill order.
	bit := uint(br.cur>>br.nbits) & 1
	return bit, nil
}
