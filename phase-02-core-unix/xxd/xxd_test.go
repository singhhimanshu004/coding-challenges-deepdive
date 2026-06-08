// xxd_test.go — exercises both engines: forward dumping under every flag and
// the reverse parser, including a binary round-trip. Tests drive the package
// through in-memory buffers, never touching real files or os globals.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// runDump is a small helper that dumps `input` with the given config and
// returns the produced text.
func runDump(t *testing.T, input []byte, cfg config) string {
	t.Helper()
	var out bytes.Buffer
	if err := dump(bytes.NewReader(input), &out, cfg); err != nil {
		t.Fatalf("dump returned error: %v", err)
	}
	return out.String()
}

func TestDumpDefault(t *testing.T) {
	got := runDump(t, []byte("hello world.seco"), config{cols: 16, group: 2})
	want := "00000000: 6865 6c6c 6f20 776f 726c 642e 7365 636f  hello world.seco\n"
	if got != want {
		t.Errorf("default dump mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpShortLinePadding(t *testing.T) {
	// A short final line must pad the hex columns so the ASCII gutter aligns.
	got := runDump(t, []byte("hi"), config{cols: 16, group: 2})
	want := "00000000: 6869                                     hi\n"
	if got != want {
		t.Errorf("short line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpColumns(t *testing.T) {
	// -c 8 puts eight bytes per line, so 10 bytes span two lines.
	got := runDump(t, []byte("0123456789"), config{cols: 8, group: 2})
	want := "00000000: 3031 3233 3435 3637  01234567\n" +
		"00000008: 3839                 89\n"
	if got != want {
		t.Errorf("-c 8 mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpGroup(t *testing.T) {
	// -g 1 separates every single byte; -g 4 packs four bytes per group.
	gotG1 := runDump(t, []byte("ABCD"), config{cols: 16, group: 1})
	wantG1 := "00000000: 41 42 43 44                                      ABCD\n"
	if gotG1 != wantG1 {
		t.Errorf("-g 1 mismatch:\n got: %q\nwant: %q", gotG1, wantG1)
	}

	gotG4 := runDump(t, []byte("ABCD"), config{cols: 16, group: 4})
	wantG4 := "00000000: 41424344                             ABCD\n"
	if gotG4 != wantG4 {
		t.Errorf("-g 4 mismatch:\n got: %q\nwant: %q", gotG4, wantG4)
	}
}

func TestDumpLength(t *testing.T) {
	// -l 5 stops after five bytes even though more input is available.
	got := runDump(t, []byte("0123456789abcdef"), config{cols: 16, group: 2, length: 5, hasLength: true})
	want := "00000000: 3031 3233 34                             01234\n"
	if got != want {
		t.Errorf("-l 5 mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpSeek(t *testing.T) {
	// -s 4 skips four bytes and starts the offset column at 0x04.
	got := runDump(t, []byte("0123456789"), config{cols: 16, group: 2, seek: 4})
	want := "00000004: 3435 3637 3839                           456789\n"
	if got != want {
		t.Errorf("-s 4 mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpNonPrintable(t *testing.T) {
	// Bytes outside the printable ASCII range render as '.' in the gutter.
	got := runDump(t, []byte{0x00, 0x09, 0x41, 0x7f, 0xff}, config{cols: 16, group: 2})
	want := "00000000: 0009 417f ff                             ..A..\n"
	if got != want {
		t.Errorf("non-printable mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestDumpEmpty(t *testing.T) {
	got := runDump(t, nil, config{cols: 16, group: 2})
	if got != "" {
		t.Errorf("empty input should produce no output, got %q", got)
	}
}

func TestReverse(t *testing.T) {
	dumpText := "00000000: 6865 6c6c 6f0a                           hello.\n"
	var out bytes.Buffer
	if err := reverse(strings.NewReader(dumpText), &out); err != nil {
		t.Fatalf("reverse error: %v", err)
	}
	if got := out.String(); got != "hello\n" {
		t.Errorf("reverse mismatch: got %q want %q", got, "hello\n")
	}
}

func TestReverseEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := reverse(strings.NewReader(""), &out); err != nil {
		t.Fatalf("reverse error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("reverse of empty input should be empty, got %q", out.String())
	}
}

// TestRoundTrip is the headline guarantee: dumping binary data and reversing it
// reproduces the original bytes exactly, NULs and high bytes included.
func TestRoundTrip(t *testing.T) {
	original := make([]byte, 256)
	for i := range original {
		original[i] = byte(i) // every possible byte value 0x00..0xff
	}
	original = append(original, []byte("trailing text\ttabs and spaces")...)

	for _, cfg := range []config{
		{cols: 16, group: 2},
		{cols: 8, group: 1},
		{cols: 24, group: 4},
	} {
		var dumped, restored bytes.Buffer
		if err := dump(bytes.NewReader(original), &dumped, cfg); err != nil {
			t.Fatalf("dump error: %v", err)
		}
		if err := reverse(bytes.NewReader(dumped.Bytes()), &restored); err != nil {
			t.Fatalf("reverse error: %v", err)
		}
		if !bytes.Equal(restored.Bytes(), original) {
			t.Errorf("round-trip mismatch for cfg %+v: got %d bytes, want %d",
				cfg, restored.Len(), len(original))
		}
	}
}

// TestCLIStdin drives the full cli() entry point with stdin, the way the real
// program is invoked with no file operand.
func TestCLIStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli(nil, strings.NewReader("hello"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cli exit = %d, stderr = %q", code, stderr.String())
	}
	want := "00000000: 6865 6c6c 6f                             hello\n"
	if stdout.String() != want {
		t.Errorf("cli stdin mismatch:\n got: %q\nwant: %q", stdout.String(), want)
	}
}

// TestCLIReverseStdin checks the -r path through cli() and a full round trip.
func TestCLIReverseStdin(t *testing.T) {
	var dumpOut, revOut, stderr bytes.Buffer
	if code := cli(nil, strings.NewReader("round trip!"), &dumpOut, &stderr); code != 0 {
		t.Fatalf("dump cli exit = %d", code)
	}
	if code := cli([]string{"-r"}, strings.NewReader(dumpOut.String()), &revOut, &stderr); code != 0 {
		t.Fatalf("reverse cli exit = %d, stderr = %q", code, stderr.String())
	}
	if revOut.String() != "round trip!" {
		t.Errorf("cli round-trip mismatch: got %q", revOut.String())
	}
}

func TestCLIBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cli([]string{"-z"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Errorf("bad flag exit = %d, want 2", code)
	}
}
