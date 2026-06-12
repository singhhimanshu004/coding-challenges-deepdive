package main

// resp.go — a hand-rolled encoder/decoder for RESP2, the REdis Serialization
// Protocol. This file is the heart of the "protocol" lesson: everything a Redis
// client and server exchange is one of five framed payloads defined here.
//
// 🐍 For a Python dev: think of this as a tiny `struct`/`pickle` pair, except the
// wire format is human-readable ASCII with `\r\n` (CRLF) terminators. There is no
// third-party library doing the work — we read and write the bytes ourselves.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// RESP2 type bytes. Every payload on the wire begins with exactly one of these
// as its first byte, which tells the parser how to read the rest of the frame.
const (
	typeSimpleString byte = '+' // +OK\r\n
	typeError        byte = '-' // -ERR something went wrong\r\n
	typeInteger      byte = ':' // :1000\r\n
	typeBulkString   byte = '$' // $5\r\nhello\r\n   (length-prefixed, binary-safe)
	typeArray        byte = '*' // *2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n
)

// crlf is the universal RESP line terminator.
const crlf = "\r\n"

// Value is a single decoded (or to-be-encoded) RESP value. Rather than a Go
// interface tree, we use one tagged struct — it keeps the encoder a simple
// switch and is easy to construct in tests.
//
// 🐍 This is the Go equivalent of a tagged union / dataclass with a `kind` field.
type Value struct {
	typ   byte    // one of the type* constants above
	str   string  // payload for simple string, error, and bulk string
	num   int64   // payload for integer
	array []Value // payload for array
	null  bool    // true => "null bulk string" ($-1) or "null array" (*-1)
}

// --- Constructors. Using helpers keeps command handlers readable. ---

// SimpleString builds a "+..." reply (short, non-binary status lines like "OK").
func SimpleString(s string) Value { return Value{typ: typeSimpleString, str: s} }

// ErrorVal builds a "-..." reply. By convention the first word is an error code
// (e.g. "ERR", "WRONGTYPE") followed by a human message.
func ErrorVal(s string) Value { return Value{typ: typeError, str: s} }

// Integer builds a ":..." reply.
func Integer(n int64) Value { return Value{typ: typeInteger, num: n} }

// BulkString builds a "$..." reply — the binary-safe string type used for almost
// all real data (GET results, ECHO payloads, …).
func BulkString(s string) Value { return Value{typ: typeBulkString, str: s} }

// NullBulk is the "$-1\r\n" reply: the canonical "key does not exist" answer.
func NullBulk() Value { return Value{typ: typeBulkString, null: true} }

// Array builds a "*..." reply from the given elements.
func Array(items ...Value) Value { return Value{typ: typeArray, array: items} }

// NullArray is the "*-1\r\n" reply (used by some commands for "no result").
func NullArray() Value { return Value{typ: typeArray, null: true} }

// Marshal serialises a Value to its RESP wire bytes. This is the encoder.
//
// We append into a byte slice rather than writing to an io.Writer so the result
// is easy to assert on byte-for-byte in unit tests. The server writes the bytes
// to the socket in one Write call.
func (v Value) Marshal() []byte {
	var buf []byte
	return v.marshalInto(buf)
}

func (v Value) marshalInto(buf []byte) []byte {
	switch v.typ {
	case typeSimpleString:
		buf = append(buf, typeSimpleString)
		buf = append(buf, v.str...)
		buf = append(buf, crlf...)
	case typeError:
		buf = append(buf, typeError)
		buf = append(buf, v.str...)
		buf = append(buf, crlf...)
	case typeInteger:
		buf = append(buf, typeInteger)
		buf = strconv.AppendInt(buf, v.num, 10)
		buf = append(buf, crlf...)
	case typeBulkString:
		if v.null {
			// Null bulk string: "$-1\r\n" — note there is no data and no trailing
			// CRLF after the length line. This is how Redis says "nil".
			buf = append(buf, "$-1"...)
			buf = append(buf, crlf...)
			break
		}
		// "$<len>\r\n<bytes>\r\n". The length is the byte count, making bulk
		// strings binary-safe: the payload may contain CRLF, NUL, anything.
		buf = append(buf, typeBulkString)
		buf = strconv.AppendInt(buf, int64(len(v.str)), 10)
		buf = append(buf, crlf...)
		buf = append(buf, v.str...)
		buf = append(buf, crlf...)
	case typeArray:
		if v.null {
			buf = append(buf, "*-1"...)
			buf = append(buf, crlf...)
			break
		}
		// "*<count>\r\n" followed by <count> nested encoded values.
		buf = append(buf, typeArray)
		buf = strconv.AppendInt(buf, int64(len(v.array)), 10)
		buf = append(buf, crlf...)
		for _, item := range v.array {
			buf = item.marshalInto(buf)
		}
	default:
		// Should be unreachable for values we construct ourselves.
		panic(fmt.Sprintf("resp: cannot marshal unknown type byte %q", v.typ))
	}
	return buf
}

// ErrProtocol is returned for malformed input that violates RESP framing.
var ErrProtocol = errors.New("resp: protocol error")

// DecodeValue reads exactly one RESP value from r. This is the decoder, used both
// to parse incoming client commands (always arrays of bulk strings) and — in the
// tests — to verify any value round-trips.
//
// 🐍 A *bufio.Reader is Go's buffered reader. `ReadByte`/`ReadString` here are the
// rough equivalents of reading from a buffered file object, but we drive the
// framing by hand instead of relying on a line-based protocol library.
func DecodeValue(r *bufio.Reader) (Value, error) {
	typeByte, err := r.ReadByte()
	if err != nil {
		return Value{}, err // often io.EOF when the client disconnects
	}

	switch typeByte {
	case typeSimpleString:
		line, err := readLine(r)
		if err != nil {
			return Value{}, err
		}
		return SimpleString(line), nil

	case typeError:
		line, err := readLine(r)
		if err != nil {
			return Value{}, err
		}
		return ErrorVal(line), nil

	case typeInteger:
		n, err := readInteger(r)
		if err != nil {
			return Value{}, err
		}
		return Integer(n), nil

	case typeBulkString:
		// First read the declared length, then read exactly that many bytes plus
		// the trailing CRLF. A length of -1 is the null bulk string.
		n, err := readInteger(r)
		if err != nil {
			return Value{}, err
		}
		if n == -1 {
			return NullBulk(), nil
		}
		if n < 0 {
			return Value{}, ErrProtocol
		}
		// io.ReadFull is the idiomatic "give me exactly N bytes or fail" call —
		// crucial because a single TCP read may return a short slice.
		body := make([]byte, n+2) // +2 for the framing CRLF
		if _, err := io.ReadFull(r, body); err != nil {
			return Value{}, err
		}
		if body[n] != '\r' || body[n+1] != '\n' {
			return Value{}, ErrProtocol
		}
		return BulkString(string(body[:n])), nil

	case typeArray:
		// Read the element count, then recursively decode that many values.
		n, err := readInteger(r)
		if err != nil {
			return Value{}, err
		}
		if n == -1 {
			return NullArray(), nil
		}
		if n < 0 {
			return Value{}, ErrProtocol
		}
		items := make([]Value, 0, n)
		for i := int64(0); i < n; i++ {
			item, err := DecodeValue(r)
			if err != nil {
				return Value{}, err
			}
			items = append(items, item)
		}
		return Array(items...), nil

	default:
		return Value{}, fmt.Errorf("%w: unknown type byte %q", ErrProtocol, typeByte)
	}
}

// readLine reads up to and including the next CRLF and returns the content with
// the CRLF stripped. RESP guarantees `\r\n` ends every line.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", ErrProtocol
	}
	return line[:len(line)-2], nil
}

// readInteger reads a CRLF-terminated line and parses it as a base-10 integer.
// Used for the length prefixes of bulk strings/arrays and for the ":" type.
func readInteger(r *bufio.Reader) (int64, error) {
	line, err := readLine(r)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: bad integer %q", ErrProtocol, line)
	}
	return n, nil
}

// asString returns the string payload of simple/bulk strings. Command parsing
// only ever deals in bulk strings, but accepting both keeps it forgiving.
func (v Value) asString() (string, bool) {
	switch v.typ {
	case typeBulkString, typeSimpleString:
		if v.null {
			return "", false
		}
		return v.str, true
	default:
		return "", false
	}
}
