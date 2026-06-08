package bloom

// BitSet is a compact, fixed-size array of bits packed into a slice of bytes.
//
// A Bloom filter needs to flip and test individual bits at arbitrary indices,
// but the smallest addressable unit in Go is a byte (8 bits). So we store the
// bits ourselves: bit i lives in byte i/8, at offset i%8 within that byte.
// Using 1 byte per bit instead would waste 8x the memory — and memory frugality
// is the entire point of a Bloom filter, so a packed representation matters.
type BitSet struct {
	bits []byte // backing storage; len = ceil(n/8)
	n    uint64 // number of addressable bits
}

// NewBitSet allocates a BitSet able to hold exactly n bits, all initially 0.
func NewBitSet(n uint64) *BitSet {
	// ceil(n/8) bytes. The +7 rounds up without floating point.
	numBytes := (n + 7) / 8
	return &BitSet{bits: make([]byte, numBytes), n: n}
}

// fromBytes rebuilds a BitSet around already-packed storage (used on load).
func fromBytes(bits []byte, n uint64) *BitSet {
	return &BitSet{bits: bits, n: n}
}

// Len reports how many bits the set addresses (its capacity, not the popcount).
func (b *BitSet) Len() uint64 { return b.n }

// Bytes exposes the packed backing storage for serialization. Callers must not
// mutate the returned slice.
func (b *BitSet) Bytes() []byte { return b.bits }

// Set turns the bit at index i on. Indices are taken modulo n so callers can
// pass raw hash outputs without bounds-checking first.
func (b *BitSet) Set(i uint64) {
	i %= b.n
	b.bits[i/8] |= 1 << (i % 8)
}

// Test reports whether the bit at index i is on. Like Set, i is reduced mod n.
func (b *BitSet) Test(i uint64) bool {
	i %= b.n
	return b.bits[i/8]&(1<<(i%8)) != 0
}

// Count returns the number of bits currently set to 1 (the population count).
// It is used to estimate the true fill ratio and false-positive probability.
func (b *BitSet) Count() uint64 {
	var total uint64
	for _, by := range b.bits {
		total += uint64(popcount(by))
	}
	return total
}

// popcount counts the set bits in a single byte via the classic
// Kernighan loop: x &= x-1 clears the lowest set bit each iteration.
func popcount(x byte) int {
	count := 0
	for x != 0 {
		x &= x - 1
		count++
	}
	return count
}
