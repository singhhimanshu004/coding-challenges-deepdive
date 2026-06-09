package main

import (
	"strings"
	"testing"
)

// --- JSON parsing tests ----------------------------------------------------

func TestParseJSONScalars(t *testing.T) {
	vals, err := ParseJSONStream(`null true false 42 3.5 "hi"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(vals) != 6 {
		t.Fatalf("expected 6 values, got %d", len(vals))
	}
	if vals[0] != nil || vals[1] != true || vals[2] != false {
		t.Errorf("scalar mismatch: %v", vals[:3])
	}
	if vals[3].(float64) != 42 || vals[4].(float64) != 3.5 || vals[5].(string) != "hi" {
		t.Errorf("scalar mismatch: %v", vals[3:])
	}
}

func TestParseJSONNested(t *testing.T) {
	vals, err := ParseJSONStream(`{"a":[1,2,{"b":null}],"c":"x"}`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	obj := vals[0].(*Object)
	if obj.Len() != 2 {
		t.Fatalf("expected 2 keys, got %d", obj.Len())
	}
	arr, _ := obj.Get("a")
	if len(arr.([]any)) != 3 {
		t.Errorf("expected 3 elements in a")
	}
}

func TestParseJSONPreservesKeyOrder(t *testing.T) {
	vals, _ := ParseJSONStream(`{"z":1,"a":2,"m":3}`)
	obj := vals[0].(*Object)
	want := []string{"z", "a", "m"}
	for i, k := range obj.Keys() {
		if k != want[i] {
			t.Errorf("key order: got %v want %v", obj.Keys(), want)
			break
		}
	}
}

func TestParseJSONUnicodeEscape(t *testing.T) {
	vals, err := ParseJSONStream(`"\u0041\u00e9"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if vals[0].(string) != "Aé" {
		t.Errorf("unicode escape: got %q", vals[0])
	}
}

func TestParseJSONErrors(t *testing.T) {
	for _, in := range []string{`{`, `[1,2`, `{"a"}`, `tru`, `"unterminated`} {
		if _, err := ParseJSONStream(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

// --- helper to run a filter against one JSON input -------------------------

func runFilter(t *testing.T, filter, jsonInput string) []any {
	t.Helper()
	prog, err := ParseFilter(filter)
	if err != nil {
		t.Fatalf("ParseFilter(%q) error: %v", filter, err)
	}
	vals, err := ParseJSONStream(jsonInput)
	if err != nil {
		t.Fatalf("bad json input: %v", err)
	}
	out, err := eval(prog, vals[0])
	if err != nil {
		t.Fatalf("eval(%q) error: %v", filter, err)
	}
	return out
}

// compact renders a value for easy comparison in tests.
func compact(v any) string {
	return encodeValue(v, encodeOptions{compact: true})
}

// --- filter feature tests --------------------------------------------------

func TestIdentity(t *testing.T) {
	out := runFilter(t, ".", `{"a":1}`)
	if got := compact(out[0]); got != `{"a":1}` {
		t.Errorf("identity: got %s", got)
	}
}

func TestFieldAccess(t *testing.T) {
	out := runFilter(t, ".name", `{"name":"Ada"}`)
	if out[0].(string) != "Ada" {
		t.Errorf("field: got %v", out[0])
	}
}

func TestNestedField(t *testing.T) {
	out := runFilter(t, ".a.b.c", `{"a":{"b":{"c":7}}}`)
	if out[0].(float64) != 7 {
		t.Errorf("nested field: got %v", out[0])
	}
}

func TestMissingFieldIsNull(t *testing.T) {
	out := runFilter(t, ".nope", `{"a":1}`)
	if out[0] != nil {
		t.Errorf("missing field should be null, got %v", out[0])
	}
}

func TestOptionalSuppressesError(t *testing.T) {
	// .foo on a number errors; the ? makes it produce nothing.
	out := runFilter(t, ".foo?", `42`)
	if len(out) != 0 {
		t.Errorf("optional should yield empty, got %v", out)
	}
}

func TestFieldOnNonObjectErrors(t *testing.T) {
	prog, _ := ParseFilter(".foo")
	vals, _ := ParseJSONStream(`42`)
	if _, err := eval(prog, vals[0]); err == nil {
		t.Errorf("expected error indexing number with field")
	}
}

func TestArrayIndex(t *testing.T) {
	out := runFilter(t, ".[1]", `[10,20,30]`)
	if out[0].(float64) != 20 {
		t.Errorf("index: got %v", out[0])
	}
}

func TestNegativeIndex(t *testing.T) {
	out := runFilter(t, ".[-1]", `[10,20,30]`)
	if out[0].(float64) != 30 {
		t.Errorf("negative index: got %v", out[0])
	}
}

func TestIndexOutOfRangeIsNull(t *testing.T) {
	out := runFilter(t, ".[9]", `[1,2]`)
	if out[0] != nil {
		t.Errorf("out-of-range index should be null, got %v", out[0])
	}
}

func TestIterateArray(t *testing.T) {
	out := runFilter(t, ".[]", `[1,2,3]`)
	if len(out) != 3 || out[2].(float64) != 3 {
		t.Errorf("iterate array: got %v", out)
	}
}

func TestIterateObject(t *testing.T) {
	out := runFilter(t, ".[]", `{"a":1,"b":2}`)
	if len(out) != 2 || out[0].(float64) != 1 || out[1].(float64) != 2 {
		t.Errorf("iterate object: got %v", out)
	}
}

func TestPipe(t *testing.T) {
	out := runFilter(t, ".a | .b", `{"a":{"b":99}}`)
	if out[0].(float64) != 99 {
		t.Errorf("pipe: got %v", out[0])
	}
}

func TestComma(t *testing.T) {
	out := runFilter(t, ".a, .b", `{"a":1,"b":2}`)
	if len(out) != 2 || out[0].(float64) != 1 || out[1].(float64) != 2 {
		t.Errorf("comma: got %v", out)
	}
}

func TestArrayConstruct(t *testing.T) {
	out := runFilter(t, "[.[] | .x]", `[{"x":1},{"x":2}]`)
	if got := compact(out[0]); got != `[1,2]` {
		t.Errorf("array construct: got %s", got)
	}
}

func TestEmptyArrayConstruct(t *testing.T) {
	out := runFilter(t, "[]", `null`)
	if got := compact(out[0]); got != `[]` {
		t.Errorf("empty array: got %s", got)
	}
}

// --- builtins --------------------------------------------------------------

func TestLength(t *testing.T) {
	cases := map[string]float64{
		`"hello"`: 5,
		`[1,2,3]`: 3,
		`{"a":1}`: 1,
		`null`:    0,
		`-7`:      7,
	}
	for in, want := range cases {
		out := runFilter(t, "length", in)
		if out[0].(float64) != want {
			t.Errorf("length(%s): got %v want %v", in, out[0], want)
		}
	}
}

func TestKeysSorted(t *testing.T) {
	out := runFilter(t, "keys", `{"z":1,"a":2}`)
	if got := compact(out[0]); got != `["a","z"]` {
		t.Errorf("keys: got %s", got)
	}
}

func TestKeysOfArray(t *testing.T) {
	out := runFilter(t, "keys", `["x","y"]`)
	if got := compact(out[0]); got != `[0,1]` {
		t.Errorf("array keys: got %s", got)
	}
}

func TestValues(t *testing.T) {
	out := runFilter(t, "values", `{"a":1,"b":2}`)
	if got := compact(out[0]); got != `[1,2]` {
		t.Errorf("values: got %s", got)
	}
}

func TestHas(t *testing.T) {
	if out := runFilter(t, `has("a")`, `{"a":1}`); out[0] != true {
		t.Errorf("has true case failed")
	}
	if out := runFilter(t, `has("z")`, `{"a":1}`); out[0] != false {
		t.Errorf("has false case failed")
	}
}

func TestSelect(t *testing.T) {
	out := runFilter(t, ".[] | select(.age > 30)", `[{"age":40},{"age":20},{"age":35}]`)
	if len(out) != 2 {
		t.Fatalf("select: expected 2 results, got %d", len(out))
	}
	first, _ := out[0].(*Object).Get("age")
	if first.(float64) != 40 {
		t.Errorf("select first: got %v", first)
	}
}

func TestMap(t *testing.T) {
	out := runFilter(t, "map(.name)", `[{"name":"a"},{"name":"b"}]`)
	if got := compact(out[0]); got != `["a","b"]` {
		t.Errorf("map: got %s", got)
	}
}

func TestMapWithArithmetic(t *testing.T) {
	out := runFilter(t, "map(. * 2)", `[1,2,3]`)
	if got := compact(out[0]); got != `[2,4,6]` {
		t.Errorf("map arithmetic: got %s", got)
	}
}

// --- comparisons & arithmetic ---------------------------------------------

func TestComparisons(t *testing.T) {
	cases := map[string]bool{
		"1 < 2":        true,
		"2 < 1":        false,
		"3 == 3":       true,
		"3 != 4":       true,
		`"a" < "b"`:    true,
		"null < false": true,
		"5 >= 5":       true,
	}
	for filter, want := range cases {
		out := runFilter(t, filter, `null`)
		if out[0] != want {
			t.Errorf("%q: got %v want %v", filter, out[0], want)
		}
	}
}

func TestArithmetic(t *testing.T) {
	out := runFilter(t, ".a + .b", `{"a":4,"b":6}`)
	if out[0].(float64) != 10 {
		t.Errorf("add: got %v", out[0])
	}
	out = runFilter(t, `"foo" + "bar"`, `null`)
	if out[0].(string) != "foobar" {
		t.Errorf("string concat: got %v", out[0])
	}
}

func TestDivideByZeroErrors(t *testing.T) {
	prog, _ := ParseFilter("1 / 0")
	if _, err := eval(prog, nil); err == nil {
		t.Errorf("expected divide-by-zero error")
	}
}

// --- composite real-world filters -----------------------------------------

func TestComposedFilter(t *testing.T) {
	in := `{"people":[{"name":"Bob","age":40},{"name":"Cy","age":25}]}`
	out := runFilter(t, ".people[] | select(.age > 30) | .name", in)
	if len(out) != 1 || out[0].(string) != "Bob" {
		t.Errorf("composed: got %v", out)
	}
}

// --- parser error cases ----------------------------------------------------

func TestFilterParseErrors(t *testing.T) {
	for _, f := range []string{".[", "| .a", "select(", "1 =", ".a.."} {
		if _, err := ParseFilter(f); err == nil {
			t.Errorf("expected parse error for %q", f)
		}
	}
}

// --- output / encoder tests ------------------------------------------------

func TestCompactOutput(t *testing.T) {
	vals, _ := ParseJSONStream(`{"a":[1,2],"b":null}`)
	if got := encodeValue(vals[0], encodeOptions{compact: true}); got != `{"a":[1,2],"b":null}` {
		t.Errorf("compact: got %s", got)
	}
}

func TestPrettyOutputIndent(t *testing.T) {
	vals, _ := ParseJSONStream(`{"a":1}`)
	want := "{\n  \"a\": 1\n}"
	if got := encodeValue(vals[0], encodeOptions{}); got != want {
		t.Errorf("pretty: got %q want %q", got, want)
	}
}

func TestSortKeysOutput(t *testing.T) {
	vals, _ := ParseJSONStream(`{"z":1,"a":2}`)
	if got := encodeValue(vals[0], encodeOptions{compact: true, sortKeys: true}); got != `{"a":2,"z":1}` {
		t.Errorf("sort keys: got %s", got)
	}
}

func TestNumberFormatting(t *testing.T) {
	if got := formatNumber(30); got != "30" {
		t.Errorf("integral number: got %s", got)
	}
	if got := formatNumber(3.5); got != "3.5" {
		t.Errorf("float number: got %s", got)
	}
}

// --- end-to-end run() tests (CLI, stdin, exit codes) -----------------------

func TestRunStdin(t *testing.T) {
	var out, errBuf strings.Builder
	code := run([]string{"-c", ".a"}, strings.NewReader(`{"a":42}`), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code: got %d, stderr=%s", code, errBuf.String())
	}
	if strings.TrimSpace(out.String()) != "42" {
		t.Errorf("stdin run: got %q", out.String())
	}
}

func TestRunRawOutput(t *testing.T) {
	var out, errBuf strings.Builder
	run([]string{"-r", ".name"}, strings.NewReader(`{"name":"Ada"}`), &out, &errBuf)
	if strings.TrimSpace(out.String()) != "Ada" {
		t.Errorf("raw output: got %q", out.String())
	}
}

func TestRunMultipleInputs(t *testing.T) {
	var out, errBuf strings.Builder
	run([]string{"-c", "."}, strings.NewReader(`1 2 3`), &out, &errBuf)
	if got := strings.TrimSpace(out.String()); got != "1\n2\n3" {
		t.Errorf("multi-input: got %q", got)
	}
}

func TestRunUsageErrorNoFilter(t *testing.T) {
	var out, errBuf strings.Builder
	if code := run([]string{}, strings.NewReader(``), &out, &errBuf); code != 2 {
		t.Errorf("expected exit 2 for missing filter, got %d", code)
	}
}

func TestRunInvalidFilter(t *testing.T) {
	var out, errBuf strings.Builder
	if code := run([]string{".["}, strings.NewReader(`{}`), &out, &errBuf); code != 2 {
		t.Errorf("expected exit 2 for bad filter, got %d", code)
	}
}

func TestRunRuntimeError(t *testing.T) {
	var out, errBuf strings.Builder
	if code := run([]string{".foo"}, strings.NewReader(`42`), &out, &errBuf); code != 1 {
		t.Errorf("expected exit 1 for runtime error, got %d", code)
	}
}

func TestRunInvalidJSON(t *testing.T) {
	var out, errBuf strings.Builder
	if code := run([]string{"."}, strings.NewReader(`{bad}`), &out, &errBuf); code != 2 {
		t.Errorf("expected exit 2 for invalid JSON, got %d", code)
	}
}
