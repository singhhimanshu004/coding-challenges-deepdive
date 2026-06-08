package bloom

import (
	"errors"
	"math"
)

// Filter is a Bloom filter: a probabilistic set that answers "have I seen this
// key?" using only a bit array and k hash functions.
//
// Guarantees:
//   - No false negatives. If a key was Added, Contains always returns true.
//   - Tunable false positives. A key that was never Added may still report true
//     with probability ~p (the rate you configured at construction).
//
// It never stores the keys themselves — only bits — which is why it can index
// millions of dictionary words in a fraction of the memory a hash set needs.
type Filter struct {
	bits *BitSet
	m    uint64 // number of bits in the array
	k    uint64 // number of hash functions
}

// New builds an empty filter sized for n expected items and a target
// false-positive rate p (e.g. 0.01 for 1%).
//
// The two sizing formulas are the classic Bloom-filter results:
//
//	m = -(n * ln p) / (ln 2)^2      // optimal number of bits
//	k = (m / n) * ln 2              // optimal number of hash functions
//
// Intuition:
//   - Smaller p demands more bits (m grows as p shrinks) — accuracy costs space.
//   - k is chosen to keep the array about half full at capacity, which is where
//     the false-positive probability is minimised. Too few hashes underuses the
//     array; too many fill it up and every lookup collides.
func New(n uint64, p float64) (*Filter, error) {
	if n == 0 {
		return nil, errors.New("bloom: expected item count n must be > 0")
	}
	if p <= 0 || p >= 1 {
		return nil, errors.New("bloom: false-positive rate p must be in (0, 1)")
	}

	m := OptimalM(n, p)
	k := OptimalK(m, n)
	return NewWithParams(m, k), nil
}

// NewWithParams builds an empty filter with explicit m and k. Used by the
// loader, which reads m and k from a saved file's header rather than recomputing
// them (the saved values are the source of truth for an existing filter).
func NewWithParams(m, k uint64) *Filter {
	if m == 0 {
		m = 1
	}
	if k == 0 {
		k = 1
	}
	return &Filter{bits: NewBitSet(m), m: m, k: k}
}

// OptimalM returns the optimal bit-array size m for n items at target rate p,
// rounded up to at least 1 bit.
func OptimalM(n uint64, p float64) uint64 {
	m := -float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)
	if m < 1 {
		return 1
	}
	return uint64(math.Ceil(m))
}

// OptimalK returns the optimal number of hash functions for m bits and n items,
// clamped to at least 1.
func OptimalK(m, n uint64) uint64 {
	k := (float64(m) / float64(n)) * math.Ln2
	if k < 1 {
		return 1
	}
	return uint64(math.Round(k))
}

// FromParts reconstructs a filter from its saved components: the bit-array size
// m, the hash count k, and the packed bit payload. Used by the codec on load so
// that an existing filter uses exactly the parameters it was built with.
func FromParts(m, k uint64, raw []byte) *Filter {
	f := NewWithParams(m, k)
	f.loadBits(raw)
	return f
}

// Add inserts a key, switching on the k bits it hashes to.
func (f *Filter) Add(key []byte) {
	for _, idx := range hashes(key, f.k, f.m) {
		f.bits.Set(idx)
	}
}

// AddString is a convenience wrapper around Add.
func (f *Filter) AddString(key string) { f.Add([]byte(key)) }

// Contains reports whether a key is *probably* present. A true result means
// "probably present" (could be a false positive); a false result means
// "definitely not present" (never a false negative).
func (f *Filter) Contains(key []byte) bool {
	for _, idx := range hashes(key, f.k, f.m) {
		// As soon as one required bit is unset, the key was definitely never
		// added — we can stop early.
		if !f.bits.Test(idx) {
			return false
		}
	}
	return true
}

// ContainsString is a convenience wrapper around Contains.
func (f *Filter) ContainsString(key string) bool { return f.Contains([]byte(key)) }

// M returns the bit-array size.
func (f *Filter) M() uint64 { return f.m }

// K returns the number of hash functions.
func (f *Filter) K() uint64 { return f.k }

// Bits exposes the underlying BitSet for serialization.
func (f *Filter) Bits() *BitSet { return f.bits }

// loadBits rebuilds a Filter's bit array from packed bytes read off disk.
func (f *Filter) loadBits(raw []byte) {
	f.bits = fromBytes(raw, f.m)
}

// EstimatedFalsePositiveRate computes the *current* false-positive probability
// from how full the array actually is, using:
//
//	p ≈ (fraction of bits set)^k
//
// This is the realised rate given the data inserted so far, which may differ
// from the configured target if the true item count differed from the estimate.
func (f *Filter) EstimatedFalsePositiveRate() float64 {
	fill := float64(f.bits.Count()) / float64(f.m)
	return math.Pow(fill, float64(f.k))
}
