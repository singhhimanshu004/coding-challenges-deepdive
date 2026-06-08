// Package codec serializes a Bloom filter to a self-describing binary file and
// loads it back. Persisting the filter means the (expensive) build step over a
// large dictionary happens once; every later spell-check just memory-maps the
// bits and runs lookups.
package codec

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"bloom/internal/bloom"
)

// File layout (all multi-byte integers big-endian):
//
//	offset  size  field
//	------  ----  ---------------------------------------------------
//	0       4     magic   "BLM1"  — identifies the format + version
//	4       1     version (1)     — bumped if the layout ever changes
//	5       8     m       uint64  — number of bits in the array
//	13      8     k       uint64  — number of hash functions
//	21      8     nbytes  uint64  — length of the packed bit payload
//	29      nbytes        — the raw bit array (ceil(m/8) bytes)
//
// Why store m and k in the header? The hash positions a key maps to depend
// entirely on m and k. A reader that guessed different values would compute
// different bit indices and every lookup would be wrong. Saving them makes the
// file self-describing: load needs nothing but the file itself.
const (
	magic   = "BLM1"
	version = 1

	headerSize = 4 + 1 + 8 + 8 + 8 // magic + version + m + k + nbytes
)

// ErrBadFormat is returned when the input is not a recognizable filter file.
var ErrBadFormat = errors.New("codec: not a valid bloom filter file")

// Save writes f to w in the BLM1 format.
func Save(w io.Writer, f *bloom.Filter) error {
	bw := bufio.NewWriter(w)

	payload := f.Bits().Bytes()

	if _, err := bw.WriteString(magic); err != nil {
		return err
	}
	if err := bw.WriteByte(version); err != nil {
		return err
	}

	var scratch [8]byte
	put := func(v uint64) error {
		binary.BigEndian.PutUint64(scratch[:], v)
		_, err := bw.Write(scratch[:])
		return err
	}
	if err := put(f.M()); err != nil {
		return err
	}
	if err := put(f.K()); err != nil {
		return err
	}
	if err := put(uint64(len(payload))); err != nil {
		return err
	}

	if _, err := bw.Write(payload); err != nil {
		return err
	}
	return bw.Flush()
}

// Load reads a BLM1 filter from r and reconstructs it. It validates the magic,
// version, and that the declared payload length matches the bytes present.
func Load(r io.Reader) (*bloom.Filter, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r, header); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, ErrBadFormat
		}
		return nil, err
	}

	if string(header[0:4]) != magic {
		return nil, ErrBadFormat
	}
	if header[4] != version {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrBadFormat, header[4])
	}

	m := binary.BigEndian.Uint64(header[5:13])
	k := binary.BigEndian.Uint64(header[13:21])
	nbytes := binary.BigEndian.Uint64(header[21:29])

	// Guard against absurd/corrupt lengths claiming gigabytes of payload.
	expected := (m + 7) / 8
	if nbytes != expected {
		return nil, fmt.Errorf("%w: payload length %d does not match m=%d", ErrBadFormat, nbytes, m)
	}

	payload := make([]byte, nbytes)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, ErrBadFormat
		}
		return nil, err
	}

	return bloom.FromParts(m, k, payload), nil
}
