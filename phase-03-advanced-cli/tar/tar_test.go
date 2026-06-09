// tar_test.go — exercises the format end to end: header round-trips, checksum
// behaviour, a full create→list→extract cycle on a real directory tree
// (including subdirs, mode and mtime preservation), path-traversal rejection,
// and the empty-archive terminator.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- Header encode/decode round-trip ----------------------------------------

func TestHeaderRoundTrip(t *testing.T) {
	orig := &header{
		Name:     "dir/sub/hello.txt",
		Mode:     0o644,
		UID:      501,
		GID:      20,
		Size:     12345,
		ModTime:  1700000000,
		TypeFlag: typeRegular,
	}

	blk, err := orig.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(blk) != blockSize {
		t.Fatalf("block size = %d, want %d", len(blk), blockSize)
	}

	got, err := decodeHeader(&blk)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Name != orig.Name || got.Mode != orig.Mode || got.UID != orig.UID ||
		got.GID != orig.GID || got.Size != orig.Size || got.ModTime != orig.ModTime ||
		got.TypeFlag != orig.TypeFlag {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", got, orig)
	}
}

// A path longer than 100 bytes must survive the prefix/name split.
func TestHeaderLongPathRoundTrip(t *testing.T) {
	long := strings.Repeat("abcdefghij/", 12) + "leaf.txt" // > 100 bytes
	orig := &header{Name: long, Mode: 0o600, Size: 3, ModTime: 1, TypeFlag: typeRegular}

	blk, err := orig.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeHeader(&blk)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != long {
		t.Fatalf("long path mismatch:\n got  %q\n want %q", got.Name, long)
	}
}

// ---- Checksum ----------------------------------------------------------------

func TestChecksumDetectsCorruption(t *testing.T) {
	h := &header{Name: "f", Mode: 0o644, Size: 1, ModTime: 1, TypeFlag: typeRegular}
	blk, _ := h.encode()

	// Flip a byte in the name field; checksum must now fail to validate.
	blk[0] ^= 0xFF
	if _, err := decodeHeader(&blk); err == nil {
		t.Fatal("expected checksum mismatch error after corrupting header")
	}
}

func TestChecksumFieldUsesSpaces(t *testing.T) {
	// Two headers identical except for an unrelated field should still each
	// validate — confirms the checksum field is excluded (treated as spaces).
	h := &header{Name: "a", Mode: 0o644, Size: 0, ModTime: 5, TypeFlag: typeRegular}
	blk, _ := h.encode()
	if _, err := decodeHeader(&blk); err != nil {
		t.Fatalf("freshly encoded header should validate: %v", err)
	}
}

// ---- Empty archive -----------------------------------------------------------

func TestEmptyArchiveTerminator(t *testing.T) {
	var buf bytes.Buffer
	aw := newArchiveWriter(&buf, false, nil)
	if err := aw.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	// An empty archive is exactly two zero blocks.
	if buf.Len() != 2*blockSize {
		t.Fatalf("empty archive size = %d, want %d", buf.Len(), 2*blockSize)
	}
	for i, b := range buf.Bytes() {
		if b != 0 {
			t.Fatalf("byte %d not zero in empty archive", i)
		}
	}

	// Listing it should yield nothing and no error.
	var out bytes.Buffer
	if err := list(&buf, false, &out); err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("listing empty archive produced output: %q", out.String())
	}
}

// ---- Full create -> list -> extract round-trip ------------------------------

func TestCreateListExtractRoundTrip(t *testing.T) {
	srcDir := t.TempDir()

	// Build a small tree:
	//   src/
	//     top.txt
	//     sub/
	//       nested.txt
	//     empty/            (directory with no files)
	writeFile(t, filepath.Join(srcDir, "top.txt"), "top-level content", 0o640)
	mkdir(t, filepath.Join(srcDir, "sub"), 0o750)
	writeFile(t, filepath.Join(srcDir, "sub", "nested.txt"), "deeply nested data here", 0o600)
	mkdir(t, filepath.Join(srcDir, "empty"), 0o700)

	// Pin a known mtime on one file so we can assert preservation precisely.
	knownMtime := time.Unix(1_650_000_000, 0)
	topPath := filepath.Join(srcDir, "top.txt")
	if err := os.Chtimes(topPath, knownMtime, knownMtime); err != nil {
		t.Fatal(err)
	}

	// --- create ---
	var archive bytes.Buffer
	aw := newArchiveWriter(&archive, false, nil)
	if err := aw.addPath(srcDir); err != nil {
		t.Fatalf("addPath: %v", err)
	}
	if err := aw.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// --- list ---
	var listing bytes.Buffer
	if err := list(bytes.NewReader(archive.Bytes()), false, &listing); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"top.txt", "sub/", "sub/nested.txt", "empty/"} {
		if !strings.Contains(listing.String(), want) {
			t.Fatalf("listing missing %q:\n%s", want, listing.String())
		}
	}

	// --- extract into a fresh dir ---
	destDir := t.TempDir()
	if err := extract(bytes.NewReader(archive.Bytes()), destDir, false, nil); err != nil {
		t.Fatalf("extract: %v", err)
	}

	// The archive stores paths with the leading "/" stripped (real tar
	// behaviour), so the tree lands under destDir at that relative path.
	rel := toArchiveName(srcDir)
	extTop := filepath.Join(destDir, rel, "top.txt")
	extNested := filepath.Join(destDir, rel, "sub", "nested.txt")

	if got := readFile(t, extTop); got != "top-level content" {
		t.Fatalf("top.txt content = %q", got)
	}
	if got := readFile(t, extNested); got != "deeply nested data here" {
		t.Fatalf("nested.txt content = %q", got)
	}

	// Mode preservation.
	if info, err := os.Stat(extTop); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o640 {
		t.Fatalf("top.txt mode = %o, want 640", info.Mode().Perm())
	}
	if info, err := os.Stat(extNested); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("nested.txt mode = %o, want 600", info.Mode().Perm())
	}

	// mtime preservation (whole-second granularity is what tar stores).
	if info, err := os.Stat(extTop); err != nil {
		t.Fatal(err)
	} else if info.ModTime().Unix() != knownMtime.Unix() {
		t.Fatalf("top.txt mtime = %d, want %d", info.ModTime().Unix(), knownMtime.Unix())
	}

	// The empty directory must exist after extraction.
	if info, err := os.Stat(filepath.Join(destDir, rel, "empty")); err != nil {
		t.Fatalf("empty dir not extracted: %v", err)
	} else if !info.IsDir() {
		t.Fatal("expected 'empty' to be a directory")
	}
}

// ---- Path traversal rejection ------------------------------------------------

func TestPathTraversalRejected(t *testing.T) {
	// Hand-craft an archive whose single entry tries to escape via "..".
	evil := &header{
		Name:     "../escape.txt",
		Mode:     0o644,
		Size:     int64(len("pwned")),
		ModTime:  1,
		TypeFlag: typeRegular,
	}
	var archive bytes.Buffer
	blk, err := evil.encode()
	if err != nil {
		t.Fatal(err)
	}
	archive.Write(blk[:])
	// Data block (padded to 512) then the terminator.
	data := make([]byte, blockSize)
	copy(data, "pwned")
	archive.Write(data)
	archive.Write(make([]byte, 2*blockSize))

	destDir := t.TempDir()
	err = extract(bytes.NewReader(archive.Bytes()), destDir, false, nil)
	if err == nil {
		t.Fatal("expected extraction to reject path traversal")
	}
	if !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("error = %v, want a path-traversal rejection", err)
	}

	// Nothing should have been written outside destDir.
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(destDir), "escape.txt")); statErr == nil {
		t.Fatal("traversal file was written outside destDir!")
	}
}

func TestAbsolutePathRejected(t *testing.T) {
	if _, err := safeJoin("/tmp/dest", "/etc/passwd"); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

// ---- Block padding -----------------------------------------------------------

func TestDataPaddedToBlock(t *testing.T) {
	srcDir := t.TempDir()
	// 600 bytes spans two blocks (512 + 88), so total data region must be 1024.
	writeFile(t, filepath.Join(srcDir, "f.bin"), strings.Repeat("x", 600), 0o644)

	var archive bytes.Buffer
	aw := newArchiveWriter(&archive, false, nil)
	if err := aw.addFile(filepath.Join(srcDir, "f.bin"), mustStat(t, filepath.Join(srcDir, "f.bin"))); err != nil {
		t.Fatal(err)
	}
	if err := aw.finish(); err != nil {
		t.Fatal(err)
	}
	// header(512) + data padded to 1024 + terminator(1024) = 2560.
	if archive.Len() != 512+1024+1024 {
		t.Fatalf("archive length = %d, want 2560 (padding wrong)", archive.Len())
	}
}

// ---- test helpers ------------------------------------------------------------

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile is subject to umask; force the exact mode.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
