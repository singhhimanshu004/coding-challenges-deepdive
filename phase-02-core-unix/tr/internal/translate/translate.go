package translate

import (
	"bufio"
	"fmt"
	"io"
)

// Spec describes a fully-parsed tr invocation: the two SETs plus the three
// mode flags. It is the small, immutable "plan" the Transformer executes.
//
// Go-for-Python note: a `struct` is a fixed bundle of named fields, like a
// Python dataclass without methods. Fields are exported (Capitalised) so the
// command layer in main.go can populate them.
type Spec struct {
	Set1       string
	Set2       string
	Delete     bool // -d : delete runes in SET1
	Squeeze    bool // -s : collapse repeated runes in the squeeze set
	Complement bool // -c : operate on the complement of SET1
}

// Transformer is the compiled, ready-to-run form of a Spec. We expand the SETs
// once up front, build fast lookup maps, and keep the tiny bit of state needed
// for squeezing (the last rune we emitted).
type Transformer struct {
	complement bool
	delete     bool
	squeeze    bool

	set1   []rune        // SET1 in original order (positions matter for translate)
	set2   []rune        // SET2 in original order
	index1 map[rune]int  // rune -> its position in set1 (for translation)
	in1    map[rune]bool // membership test for SET1
	in2    map[rune]bool // membership test for SET2

	// Squeeze state. Go zero-values these: lastEmitted = 0, haveLast = false.
	lastEmitted rune
	haveLast    bool
}

// New validates a Spec and compiles it into a Transformer, expanding ranges
// and character classes. It returns an error for nonsensical combinations
// (e.g. translation with no SET2) so the caller can report a usage error.
func New(spec Spec) (*Transformer, error) {
	set1, err := ExpandSet(spec.Set1)
	if err != nil {
		return nil, fmt.Errorf("SET1: %w", err)
	}
	set2, err := ExpandSet(spec.Set2)
	if err != nil {
		return nil, fmt.Errorf("SET2: %w", err)
	}

	translating := !spec.Delete && spec.Set2 != ""
	if translating && len(set2) == 0 {
		return nil, fmt.Errorf("when translating, SET2 must not be empty")
	}
	if !spec.Delete && !spec.Squeeze && spec.Set2 == "" {
		return nil, fmt.Errorf("missing SET2 (translation needs two sets)")
	}

	t := &Transformer{
		complement: spec.Complement,
		delete:     spec.Delete,
		squeeze:    spec.Squeeze,
		set1:       set1,
		set2:       set2,
		index1:     make(map[rune]int, len(set1)),
		in1:        make(map[rune]bool, len(set1)),
		in2:        make(map[rune]bool, len(set2)),
	}
	// Record the *first* position of each rune in SET1; tr maps using the
	// earliest match, mirroring GNU/BSD behaviour for repeated chars.
	for i, r := range set1 {
		if _, seen := t.index1[r]; !seen {
			t.index1[r] = i
		}
		t.in1[r] = true
	}
	for _, r := range set2 {
		t.in2[r] = true
	}
	return t, nil
}

// Run streams runes from r to w, applying the translation. It uses buffered
// I/O so we process the input rune-by-rune without one syscall per character —
// the same pipe-and-filter pattern as the other Phase 2 tools.
//
// Go-for-Python note: io.Reader / io.Writer are interfaces — any type with the
// right Read/Write method satisfies them. That's why the same function works
// for os.Stdin/os.Stdout in production and strings.Reader/bytes.Buffer in tests.
func (t *Transformer) Run(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	// `defer` schedules bw.Flush() to run when Run returns, guaranteeing the
	// buffer is drained even on an early error — like a Python finally block.
	defer bw.Flush()

	for {
		ru, _, err := br.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := t.writeRune(bw, ru); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// writeRune applies delete/translate then the optional squeeze to a single
// input rune.
func (t *Transformer) writeRune(bw *bufio.Writer, ru rune) error {
	if t.delete {
		if t.inDeleteSet(ru) {
			return nil // dropped
		}
		return t.emit(bw, ru)
	}

	if len(t.set2) > 0 {
		ru = t.translateRune(ru)
	}
	return t.emit(bw, ru)
}

// inDeleteSet reports whether ru should be deleted, honouring -c which flips
// SET1 to "every rune that is NOT in SET1".
func (t *Transformer) inDeleteSet(ru rune) bool {
	in := t.in1[ru]
	if t.complement {
		in = !in
	}
	return in
}

// translateRune maps a rune through SET1 → SET2.
//
//   - Normal: find ru's position in SET1 and emit the rune at the same index
//     in SET2; if SET2 is shorter, its last rune repeats (tr's pad rule).
//   - Complement (-c): the "matched" set is everything NOT in SET1, so any rune
//     outside SET1 becomes SET2's last rune; runes inside SET1 pass through.
func (t *Transformer) translateRune(ru rune) rune {
	last := t.set2[len(t.set2)-1]
	if t.complement {
		if t.in1[ru] {
			return ru
		}
		return last
	}
	if idx, ok := t.index1[ru]; ok {
		if idx < len(t.set2) {
			return t.set2[idx]
		}
		return last
	}
	return ru
}

// emit writes ru, unless squeeze is on and ru is a repeat of the last emitted
// rune *and* belongs to the squeeze set.
func (t *Transformer) emit(bw *bufio.Writer, ru rune) error {
	if t.squeeze && t.inSqueezeSet(ru) && t.haveLast && t.lastEmitted == ru {
		return nil // collapse the repeat
	}
	if _, err := bw.WriteRune(ru); err != nil {
		return err
	}
	t.lastEmitted = ru
	t.haveLast = true
	return nil
}

// inSqueezeSet decides which runes -s collapses. tr's rule depends on the mode:
//   - delete + squeeze:   squeeze the runes in SET2
//   - translate + squeeze: squeeze the runes in SET2 (the output alphabet)
//   - squeeze only:        squeeze the runes in SET1 (honouring -c)
func (t *Transformer) inSqueezeSet(ru rune) bool {
	if !t.squeeze {
		return false
	}
	if t.delete || len(t.set2) > 0 {
		return t.in2[ru]
	}
	in := t.in1[ru]
	if t.complement {
		in = !in
	}
	return in
}
