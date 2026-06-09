package main

// eval.go — the tree-walking evaluator. This is the third stage of the pipeline
// (lex → parse → **evaluate**) and the conceptual heart of jq.
//
// The core signature is:
//
//	func eval(node Node, input any) ([]any, error)
//
// Every jq expression is a *function from one input value to a stream of output
// values*. We model "a stream" as a Go slice `[]any`. Identity returns one
// value; `.[]` returns many; `select(...)` may return zero. The pipe `|` is
// literally "feed each of my left output into my right".
//
// 🐍 Python analogy: think of each node as a generator function
// `def f(input): yield ...`. Real jq is lazily streaming; we eagerly collect
// into slices to keep the code beginner-readable. (Trade-off noted in README.)

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// eval dispatches on the concrete node type. A Go type switch is the idiomatic
// replacement for the visitor pattern you might use in Java/Python here.
func eval(node Node, input any) ([]any, error) {
	switch n := node.(type) {
	case Identity:
		return []any{input}, nil

	case Field:
		return evalField(n, input)

	case Index:
		return evalIndex(n, input)

	case Iterate:
		return evalIterate(input)

	case Pipe:
		return evalPipe(n, input)

	case Comma:
		left, err := eval(n.Left, input)
		if err != nil {
			return nil, err
		}
		right, err := eval(n.Right, input)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil

	case ArrayConstruct:
		return evalArrayConstruct(n, input)

	case Literal:
		return []any{n.Value}, nil

	case Binary:
		return evalBinary(n, input)

	case Call:
		return evalCall(n, input)

	case Try:
		out, err := eval(n.Expr, input)
		if err != nil {
			return []any{}, nil // swallow the error → empty stream
		}
		return out, nil

	default:
		return nil, fmt.Errorf("internal error: unknown node type %T", node)
	}
}

// evalField implements `.name`. On an object it returns the value (or null if
// the key is absent); on null it returns null (jq treats null as a soft value);
// anything else is an error (unless wrapped in `?`).
func evalField(n Field, input any) ([]any, error) {
	switch v := input.(type) {
	case *Object:
		val, _ := v.Get(n.Name) // missing key → nil, which is exactly null
		return []any{val}, nil
	case nil:
		return []any{nil}, nil
	default:
		return nil, fmt.Errorf("cannot index %s with %q", typeName(input), n.Name)
	}
}

// evalIndex implements `.[expr]`. The index expression itself is a filter, so it
// can produce several indices (e.g. `.[0,1]`); we take the cartesian product.
func evalIndex(n Index, input any) ([]any, error) {
	idxs, err := eval(n.Index, input)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, idx := range idxs {
		switch container := input.(type) {
		case []any:
			f, ok := idx.(float64)
			if !ok {
				return nil, fmt.Errorf("cannot index array with %s", typeName(idx))
			}
			i := int(f)
			if i < 0 {
				i += len(container) // negative indices count from the end
			}
			if i < 0 || i >= len(container) {
				out = append(out, nil) // out of range → null, like jq
			} else {
				out = append(out, container[i])
			}
		case *Object:
			key, ok := idx.(string)
			if !ok {
				return nil, fmt.Errorf("cannot index object with %s", typeName(idx))
			}
			val, _ := container.Get(key)
			out = append(out, val)
		case nil:
			out = append(out, nil)
		default:
			return nil, fmt.Errorf("cannot index %s", typeName(input))
		}
	}
	return out, nil
}

// evalIterate implements `.[]` — explode an array into its elements or an object
// into its values, producing a multi-value stream.
func evalIterate(input any) ([]any, error) {
	switch v := input.(type) {
	case []any:
		return append([]any{}, v...), nil
	case *Object:
		out := make([]any, 0, v.Len())
		for _, k := range v.Keys() {
			val, _ := v.Get(k)
			out = append(out, val)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cannot iterate over %s", typeName(input))
	}
}

// evalPipe implements `left | right`: run left, then feed every value it
// produced into right, concatenating right's outputs. This is where the
// streaming nature of jq becomes visible.
func evalPipe(n Pipe, input any) ([]any, error) {
	lefts, err := eval(n.Left, input)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, v := range lefts {
		rights, err := eval(n.Right, v)
		if err != nil {
			return nil, err
		}
		out = append(out, rights...)
	}
	return out, nil
}

// evalArrayConstruct implements `[ expr ]`: collect *all* of expr's outputs into
// a single new array value. `[]` builds an empty array.
func evalArrayConstruct(n ArrayConstruct, input any) ([]any, error) {
	if n.Expr == nil {
		return []any{[]any{}}, nil
	}
	vals, err := eval(n.Expr, input)
	if err != nil {
		return nil, err
	}
	arr := make([]any, 0, len(vals))
	arr = append(arr, vals...)
	return []any{arr}, nil
}

// evalBinary implements arithmetic and comparison. Both operands are filters, so
// we take the cartesian product of their output streams (jq semantics).
func evalBinary(n Binary, input any) ([]any, error) {
	lefts, err := eval(n.Left, input)
	if err != nil {
		return nil, err
	}
	rights, err := eval(n.Right, input)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, l := range lefts {
		for _, r := range rights {
			v, err := applyBinary(n.Op, l, r)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func applyBinary(op tokenType, l, r any) (any, error) {
	switch op {
	case tEq:
		return equal(l, r), nil
	case tNe:
		return !equal(l, r), nil
	case tLt:
		return compare(l, r) < 0, nil
	case tGt:
		return compare(l, r) > 0, nil
	case tLe:
		return compare(l, r) <= 0, nil
	case tGe:
		return compare(l, r) >= 0, nil
	case tPlus:
		return addValues(l, r)
	case tMinus, tStar, tSlash:
		lf, lok := l.(float64)
		rf, rok := r.(float64)
		if !lok || !rok {
			return nil, fmt.Errorf("cannot do arithmetic on %s and %s", typeName(l), typeName(r))
		}
		switch op {
		case tMinus:
			return lf - rf, nil
		case tStar:
			return lf * rf, nil
		default: // tSlash
			if rf == 0 {
				return nil, fmt.Errorf("cannot divide by zero")
			}
			return lf / rf, nil
		}
	}
	return nil, fmt.Errorf("internal error: unknown operator")
}

// addValues implements `+`: numeric add, string concat, array concat, and
// object merge (right wins on key clashes) — mirroring jq's overloaded `+`.
func addValues(l, r any) (any, error) {
	switch lv := l.(type) {
	case float64:
		if rv, ok := r.(float64); ok {
			return lv + rv, nil
		}
	case string:
		if rv, ok := r.(string); ok {
			return lv + rv, nil
		}
	case []any:
		if rv, ok := r.([]any); ok {
			return append(append([]any{}, lv...), rv...), nil
		}
	case *Object:
		if rv, ok := r.(*Object); ok {
			merged := NewObject()
			for _, k := range lv.Keys() {
				v, _ := lv.Get(k)
				merged.Set(k, v)
			}
			for _, k := range rv.Keys() {
				v, _ := rv.Get(k)
				merged.Set(k, v)
			}
			return merged, nil
		}
	case nil:
		return r, nil // null + x == x in jq
	}
	if r == nil {
		return l, nil
	}
	return nil, fmt.Errorf("cannot add %s and %s", typeName(l), typeName(r))
}

// evalCall dispatches the builtin functions.
func evalCall(n Call, input any) ([]any, error) {
	switch n.Name {
	case "length":
		return builtinLength(input)
	case "keys", "keys_unsorted":
		return builtinKeys(input, n.Name == "keys")
	case "values":
		return builtinValues(input)
	case "has":
		return builtinHas(n, input)
	case "select":
		return builtinSelect(n, input)
	case "map":
		return builtinMap(n, input)
	case "not":
		return []any{!truthy(input)}, nil
	case "empty":
		return []any{}, nil
	default:
		return nil, fmt.Errorf("unknown function %q", n.Name)
	}
}

func builtinLength(input any) ([]any, error) {
	switch v := input.(type) {
	case nil:
		return []any{float64(0)}, nil
	case string:
		return []any{float64(len([]rune(v)))}, nil
	case []any:
		return []any{float64(len(v))}, nil
	case *Object:
		return []any{float64(v.Len())}, nil
	case float64:
		if v < 0 {
			v = -v
		}
		return []any{v}, nil
	default:
		return nil, fmt.Errorf("%s has no length", typeName(input))
	}
}

func builtinKeys(input any, sorted bool) ([]any, error) {
	switch v := input.(type) {
	case *Object:
		keys := append([]string{}, v.Keys()...)
		if sorted {
			sort.Strings(keys)
		}
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return []any{out}, nil
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = float64(i)
		}
		return []any{out}, nil
	default:
		return nil, fmt.Errorf("%s has no keys", typeName(input))
	}
}

// builtinValues returns an array of an object's values (insertion order) or an
// array's elements.
//
// ⚠️ Divergence from real jq: jq's `values` builtin actually filters a stream,
// keeping only non-null inputs (`select(. != null)`). We implement the more
// intuitive "give me the values" meaning that the challenge brief asks for, and
// call it out here and in the README so the difference is explicit.
func builtinValues(input any) ([]any, error) {
	switch v := input.(type) {
	case *Object:
		out := make([]any, 0, v.Len())
		for _, k := range v.Keys() {
			val, _ := v.Get(k)
			out = append(out, val)
		}
		return []any{out}, nil
	case []any:
		return []any{append([]any{}, v...)}, nil
	default:
		return nil, fmt.Errorf("%s has no values", typeName(input))
	}
}

func builtinHas(n Call, input any) ([]any, error) {
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("has expects 1 argument")
	}
	keys, err := eval(n.Args[0], input)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, key := range keys {
		switch v := input.(type) {
		case *Object:
			s, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("has on object needs a string key")
			}
			_, present := v.Get(s)
			out = append(out, present)
		case []any:
			f, ok := key.(float64)
			if !ok {
				return nil, fmt.Errorf("has on array needs a number index")
			}
			i := int(f)
			out = append(out, i >= 0 && i < len(v))
		default:
			return nil, fmt.Errorf("cannot check has() on %s", typeName(input))
		}
	}
	return out, nil
}

// builtinSelect implements `select(f)`: for each value f produces, emit the
// *input* once if that value is truthy. This is the standard jq definition
// `def select(f): if f then . else empty end;`.
func builtinSelect(n Call, input any) ([]any, error) {
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("select expects 1 argument")
	}
	conds, err := eval(n.Args[0], input)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, c := range conds {
		if truthy(c) {
			out = append(out, input)
		}
	}
	return out, nil
}

// builtinMap implements `map(f)` == `[ .[] | f ]`: apply f to every element of
// the input array and collect all results into a new array.
func builtinMap(n Call, input any) ([]any, error) {
	if len(n.Args) != 1 {
		return nil, fmt.Errorf("map expects 1 argument")
	}
	arr, ok := input.([]any)
	if !ok {
		return nil, fmt.Errorf("cannot map over %s", typeName(input))
	}
	result := []any{}
	for _, elem := range arr {
		vals, err := eval(n.Args[0], elem)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}
	return []any{result}, nil
}

// --- value helpers ---------------------------------------------------------

// truthy follows jq's rule: everything is true except `false` and `null`.
func truthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// typeName gives jq-style type names for error messages.
func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case *Object:
		return "object"
	default:
		return "unknown"
	}
}

// typeRank encodes jq's cross-type ordering: null < false < true < numbers <
// strings < arrays < objects.
func typeRank(v any) int {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if !x {
			return 1
		}
		return 2
	case float64:
		return 3
	case string:
		return 4
	case []any:
		return 5
	case *Object:
		return 6
	default:
		return 7
	}
}

// compare returns -1/0/1 using jq's total ordering across all value types.
func compare(a, b any) int {
	ra, rb := typeRank(a), typeRank(b)
	if ra != rb {
		return sign(ra - rb)
	}
	switch av := a.(type) {
	case float64:
		bv := b.(float64)
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		default:
			return 0
		}
	case string:
		return strings.Compare(av, b.(string))
	case []any:
		bv := b.([]any)
		for i := 0; i < len(av) && i < len(bv); i++ {
			if c := compare(av[i], bv[i]); c != 0 {
				return c
			}
		}
		return sign(len(av) - len(bv))
	case *Object:
		// Compare by sorted keys, then by the corresponding values.
		bv := b.(*Object)
		ak := append([]string{}, av.Keys()...)
		bk := append([]string{}, bv.Keys()...)
		sort.Strings(ak)
		sort.Strings(bk)
		for i := 0; i < len(ak) && i < len(bk); i++ {
			if c := strings.Compare(ak[i], bk[i]); c != 0 {
				return c
			}
		}
		if c := sign(len(ak) - len(bk)); c != 0 {
			return c
		}
		for _, k := range ak {
			x, _ := av.Get(k)
			y, _ := bv.Get(k)
			if c := compare(x, y); c != 0 {
				return c
			}
		}
		return 0
	}
	return 0 // null / bool already fully ordered by rank
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// equal is deep equality built on compare.
func equal(a, b any) bool { return compare(a, b) == 0 }

// parseFloat is a tiny wrapper used by the filter parser for numeric literals.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
