package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestMarshal verifies the encoder produces exact RESP bytes for every type,
// including the two null forms and a nested array. Crafted-bytes assertions like
// these are the surest way to prove protocol correctness.
func TestMarshal(t *testing.T) {
	cases := []struct {
		name string
		in   Value
		want string
	}{
		{"simple string", SimpleString("OK"), "+OK\r\n"},
		{"error", ErrorVal("ERR boom"), "-ERR boom\r\n"},
		{"integer", Integer(1000), ":1000\r\n"},
		{"negative integer", Integer(-42), ":-42\r\n"},
		{"bulk string", BulkString("hello"), "$5\r\nhello\r\n"},
		{"empty bulk string", BulkString(""), "$0\r\n\r\n"},
		{"binary-safe bulk string", BulkString("a\r\nb"), "$4\r\na\r\nb\r\n"},
		{"null bulk", NullBulk(), "$-1\r\n"},
		{"null array", NullArray(), "*-1\r\n"},
		{"empty array", Array(), "*0\r\n"},
		{
			"command array",
			Array(BulkString("SET"), BulkString("k"), BulkString("v")),
			"*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n",
		},
		{
			"nested array",
			Array(Integer(1), Array(BulkString("x"), NullBulk())),
			"*2\r\n:1\r\n*2\r\n$1\r\nx\r\n$-1\r\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(tc.in.Marshal())
			if got != tc.want {
				t.Fatalf("Marshal() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDecodeValue verifies the decoder parses every type from crafted bytes.
func TestDecodeValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Value
	}{
		{"simple string", "+OK\r\n", SimpleString("OK")},
		{"error", "-ERR boom\r\n", ErrorVal("ERR boom")},
		{"integer", ":1000\r\n", Integer(1000)},
		{"negative integer", ":-7\r\n", Integer(-7)},
		{"bulk string", "$5\r\nhello\r\n", BulkString("hello")},
		{"empty bulk string", "$0\r\n\r\n", BulkString("")},
		{"binary-safe bulk string", "$4\r\na\r\nb\r\n", BulkString("a\r\nb")},
		{"null bulk", "$-1\r\n", NullBulk()},
		{"null array", "*-1\r\n", NullArray()},
		{"empty array", "*0\r\n", Array()},
		{
			"command array",
			"*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n",
			Array(BulkString("SET"), BulkString("k"), BulkString("v")),
		},
		{
			"nested array",
			"*2\r\n:1\r\n*2\r\n$1\r\nx\r\n$-1\r\n",
			Array(Integer(1), Array(BulkString("x"), NullBulk())),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.in))
			got, err := DecodeValue(r)
			if err != nil {
				t.Fatalf("DecodeValue() error = %v", err)
			}
			if !valuesEqual(got, tc.want) {
				t.Fatalf("DecodeValue() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestRoundTrip is a property-style check: encoding then decoding any value must
// reproduce it. This catches asymmetries between the two halves of the protocol.
func TestRoundTrip(t *testing.T) {
	values := []Value{
		SimpleString("PONG"),
		ErrorVal("WRONGTYPE nope"),
		Integer(0),
		Integer(-123456789),
		BulkString(""),
		BulkString("the quick brown fox"),
		BulkString("contains\r\nCRLF\x00and NUL"),
		NullBulk(),
		NullArray(),
		Array(),
		Array(BulkString("MSET"), BulkString("a"), BulkString("1"), BulkString("b"), BulkString("2")),
		Array(Integer(1), Array(NullBulk(), BulkString("deep"))),
	}
	for _, v := range values {
		encoded := v.Marshal()
		r := bufio.NewReader(bytes.NewReader(encoded))
		decoded, err := DecodeValue(r)
		if err != nil {
			t.Fatalf("round trip of %#v: decode error %v", v, err)
		}
		if !valuesEqual(v, decoded) {
			t.Fatalf("round trip mismatch: got %#v, want %#v", decoded, v)
		}
	}
}

// TestDecodeErrors verifies malformed frames are rejected rather than silently
// accepted.
func TestDecodeErrors(t *testing.T) {
	bad := []string{
		"$5\r\nhi\r\n",          // declared length 5 but only 2 bytes of body
		":notanumber\r\n",       // integer that isn't
		"+missing terminator\n", // LF without CR
		"?unknown type\r\n",     // unknown leading byte
	}
	for _, in := range bad {
		r := bufio.NewReader(strings.NewReader(in))
		if _, err := DecodeValue(r); err == nil {
			t.Fatalf("expected error decoding %q, got nil", in)
		}
	}
}

// valuesEqual compares two Values structurally (Go has no built-in deep-equal that
// reads nicely for this tagged struct, and reflect.DeepEqual trips on unexported
// zero fields, so we compare explicitly).
func valuesEqual(a, b Value) bool {
	if a.typ != b.typ || a.null != b.null {
		return false
	}
	switch a.typ {
	case typeSimpleString, typeError, typeBulkString:
		return a.str == b.str
	case typeInteger:
		return a.num == b.num
	case typeArray:
		if len(a.array) != len(b.array) {
			return false
		}
		for i := range a.array {
			if !valuesEqual(a.array[i], b.array[i]) {
				return false
			}
		}
		return true
	}
	return false
}
