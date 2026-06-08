package bloom

import "hash/fnv"

// baseHashes produces two independent 64-bit hash values for a single key.
//
// A Bloom filter needs k independent hash functions, but writing (and running)
// k genuinely different hash algorithms is wasteful. The Kirsch–Mitzenmacher
// technique proves you can synthesise as many hashes as you like from just two:
//
//	g_i(x) = h1(x) + i*h2(x)   for i = 0, 1, ..., k-1
//
// The resulting family behaves, for Bloom-filter purposes, statistically like k
// independent hashes — with no measurable increase in false-positive rate. This
// is "double hashing".
//
// We derive h1 and h2 from a single 64-bit FNV-1a digest by splitting it into
// its high and low 32-bit halves. FNV-1a is chosen because it is tiny, has no
// dependencies, is fast on short strings (dictionary words), and disperses bits
// well enough for this use. It is NOT cryptographic — that is fine here; we only
// need good distribution, not collision resistance against an adversary.
func baseHashes(data []byte) (h1, h2 uint64) {
	h := fnv.New64a()
	h.Write(data)
	sum := h.Sum64()

	h1 = sum >> 32           // high 32 bits
	h2 = sum & 0xffffffff    // low 32 bits
	// If h2 were 0, every derived hash would collapse to h1 (i*0 == 0), so the
	// filter would effectively use a single bit position. Force it non-zero.
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

// hashes returns the k bit indices a key maps to, each already reduced into
// [0, m). This is the heart of both insertion and lookup: Add sets every
// returned index, Test checks every returned index.
func hashes(data []byte, k uint64, m uint64) []uint64 {
	h1, h2 := baseHashes(data)
	out := make([]uint64, k)
	for i := uint64(0); i < k; i++ {
		// g_i(x) = h1 + i*h2, folded into the bit array's range.
		out[i] = (h1 + i*h2) % m
	}
	return out
}
