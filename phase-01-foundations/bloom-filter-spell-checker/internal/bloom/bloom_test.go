package bloom

import (
	"fmt"
	"math"
	"testing"
)

// TestNoFalseNegatives is the core Bloom-filter guarantee: every key that was
// inserted MUST report present. There is no acceptable failure rate here — a
// single miss would be a correctness bug.
func TestNoFalseNegatives(t *testing.T) {
	const n = 5000
	f, err := New(n, 0.01)
	if err != nil {
		t.Fatal(err)
	}

	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("word-%d", i)
		f.AddString(words[i])
	}

	for _, w := range words {
		if !f.ContainsString(w) {
			t.Fatalf("false negative: inserted %q but Contains reported false", w)
		}
	}
}

// TestFalsePositiveRateNearTarget inserts n items, then probes m items that were
// never inserted. The observed false-positive rate should land close to the
// configured target p. We allow generous slack (3x) because it is a statistical
// estimate, not an exact value — the point is to prove the math is in the right
// ballpark, not to flake on noise.
func TestFalsePositiveRateNearTarget(t *testing.T) {
	const (
		n      = 10000
		probes = 20000
		target = 0.01
	)
	f, err := New(n, target)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		f.AddString(fmt.Sprintf("present-%d", i))
	}

	falsePositives := 0
	for i := 0; i < probes; i++ {
		// "absent-*" keys are disjoint from the "present-*" keys inserted above.
		if f.ContainsString(fmt.Sprintf("absent-%d", i)) {
			falsePositives++
		}
	}
	observed := float64(falsePositives) / float64(probes)

	t.Logf("target p=%.4f, observed FP rate=%.4f (%d/%d)", target, observed, falsePositives, probes)
	if observed > target*3 {
		t.Fatalf("observed FP rate %.4f far exceeds target %.4f", observed, target)
	}
}

func TestOptimalParams(t *testing.T) {
	// Spot-check the textbook example: n=1,000,000 items at p=0.01 needs roughly
	// 9.6 million bits and 7 hash functions.
	m := OptimalM(1_000_000, 0.01)
	k := OptimalK(m, 1_000_000)
	if m < 9_000_000 || m > 10_000_000 {
		t.Fatalf("m = %d, expected ~9.6 million", m)
	}
	if k != 7 {
		t.Fatalf("k = %d, expected 7", k)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(0, 0.01); err == nil {
		t.Fatal("n=0 should be rejected")
	}
	for _, p := range []float64{0, 1, -0.5, 2} {
		if _, err := New(100, p); err == nil {
			t.Fatalf("p=%g should be rejected", p)
		}
	}
}

// TestSingleWord — the smallest meaningful dictionary. The one word must be
// present; an unrelated word should (almost certainly) be absent.
func TestSingleWord(t *testing.T) {
	f, err := New(1, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	f.AddString("solitude")
	if !f.ContainsString("solitude") {
		t.Fatal("the single inserted word must be present")
	}
	if f.ContainsString("crowd") {
		t.Fatal("an unrelated word should not be present in a 1-word filter")
	}
}

func TestParamsAreClamped(t *testing.T) {
	// Degenerate params must not produce a zero-sized array or zero hashes.
	f := NewWithParams(0, 0)
	if f.M() < 1 || f.K() < 1 {
		t.Fatalf("params not clamped: m=%d k=%d", f.M(), f.K())
	}
	f.AddString("x")
	if !f.ContainsString("x") {
		t.Fatal("clamped filter should still function")
	}
}

func TestEstimatedFalsePositiveRate(t *testing.T) {
	f, err := New(1000, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	// Empty filter: nothing set, so the estimate is 0.
	if got := f.EstimatedFalsePositiveRate(); got != 0 {
		t.Fatalf("empty filter estimate = %g, want 0", got)
	}
	for i := 0; i < 1000; i++ {
		f.AddString(fmt.Sprintf("w%d", i))
	}
	// At capacity the estimate should be in a sane neighbourhood of the target.
	if got := f.EstimatedFalsePositiveRate(); math.IsNaN(got) || got < 0 || got > 0.1 {
		t.Fatalf("estimate at capacity = %g, out of expected range", got)
	}
}
