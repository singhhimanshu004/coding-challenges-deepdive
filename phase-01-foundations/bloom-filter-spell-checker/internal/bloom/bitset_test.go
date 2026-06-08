package bloom

import "testing"

func TestBitSetSetAndTest(t *testing.T) {
	b := NewBitSet(64)

	// Nothing set initially.
	for i := uint64(0); i < 64; i++ {
		if b.Test(i) {
			t.Fatalf("bit %d should be unset on a fresh BitSet", i)
		}
	}

	// Set a scattered handful, including byte boundaries (7,8) and the last bit.
	for _, i := range []uint64{0, 1, 7, 8, 31, 63} {
		b.Set(i)
	}
	for _, i := range []uint64{0, 1, 7, 8, 31, 63} {
		if !b.Test(i) {
			t.Fatalf("bit %d should be set", i)
		}
	}
	// A neighbour that was not set must remain unset (no accidental spillover).
	if b.Test(2) {
		t.Fatal("bit 2 should still be unset")
	}
}

func TestBitSetCount(t *testing.T) {
	b := NewBitSet(100)
	if b.Count() != 0 {
		t.Fatalf("empty Count = %d, want 0", b.Count())
	}
	indices := []uint64{3, 3, 3, 10, 99} // duplicates must not double-count
	for _, i := range indices {
		b.Set(i)
	}
	if got, want := b.Count(), uint64(3); got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
}

func TestBitSetModWraps(t *testing.T) {
	// Index >= n must wrap rather than panic — Add/Test rely on this.
	b := NewBitSet(10)
	b.Set(13) // 13 % 10 == 3
	if !b.Test(3) {
		t.Fatal("index 13 should wrap to bit 3")
	}
	if !b.Test(13) {
		t.Fatal("Test(13) should also wrap to bit 3")
	}
}

func TestBitSetByteSizing(t *testing.T) {
	// 10 bits needs ceil(10/8) = 2 bytes.
	b := NewBitSet(10)
	if len(b.Bytes()) != 2 {
		t.Fatalf("10-bit set uses %d bytes, want 2", len(b.Bytes()))
	}
	// 16 bits needs exactly 2 bytes (no off-by-one rounding up to 3).
	if got := len(NewBitSet(16).Bytes()); got != 2 {
		t.Fatalf("16-bit set uses %d bytes, want 2", got)
	}
}
