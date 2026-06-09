// writer.go — the CREATE half of tar (`-c`). It walks the files and
// directories you name on the command line and emits a stream of
// [header block][data blocks][padding] records, finished with the two
// all-zero blocks that mark end-of-archive.
//
// 🐍➡️🐹 Python analogy: this is the equivalent of `tarfile.open(mode="w")`
// plus `tar.add(path)` — but here we hand-write each 512-byte record instead
// of leaning on a library.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// archiveWriter streams tar records to an underlying writer (a file or, in
// tests, a bytes.Buffer). Holding the io.Writer as an interface is the Go
// idiom that makes the writer testable without ever touching the disk.
type archiveWriter struct {
	w       io.Writer
	verbose bool
	log     io.Writer // where `-v` names are printed (usually stderr)
}

// newArchiveWriter wires up a writer. `verbose` toggles the `-v` listing.
func newArchiveWriter(w io.Writer, verbose bool, log io.Writer) *archiveWriter {
	return &archiveWriter{w: w, verbose: verbose, log: log}
}

// addPath archives a single path. If the path is a directory we recurse into
// it, emitting the directory entry first and then its children — exactly the
// order real tar uses so extraction can recreate parents before their files.
func (aw *archiveWriter) addPath(path string) error {
	info, err := os.Lstat(path) // Lstat: don't follow symlinks at the top level
	if err != nil {
		return err
	}

	if info.IsDir() {
		return aw.addDir(path)
	}
	return aw.addFile(path, info)
}

// addDir writes the directory's own header, then walks its contents in sorted
// order (deterministic archives make tests and diffs reproducible).
func (aw *archiveWriter) addDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if err := aw.writeDirHeader(path, info); err != nil {
		return err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	// os.ReadDir already returns entries sorted by name, but we sort again to
	// be explicit about the guarantee the rest of the code relies on.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		child := filepath.Join(path, e.Name())
		if err := aw.addPath(child); err != nil {
			return err
		}
	}
	return nil
}

// writeDirHeader emits a single directory record. Directories carry no data,
// so size is 0 and there are no data blocks — just the header.
func (aw *archiveWriter) writeDirHeader(path string, info os.FileInfo) error {
	name := toArchiveName(path)
	// Directory names conventionally end in "/" so listings and other tools
	// can tell them apart at a glance.
	if name != "" && name[len(name)-1] != '/' {
		name += "/"
	}

	h := &header{
		Name:     name,
		Mode:     int64(info.Mode().Perm()),
		Size:     0,
		ModTime:  info.ModTime().Unix(),
		TypeFlag: typeDir,
	}
	if err := aw.writeHeader(h); err != nil {
		return err
	}
	aw.report(name)
	return nil
}

// addFile writes a regular file's header followed by its data, padded to a
// whole number of 512-byte blocks.
func (aw *archiveWriter) addFile(path string, info os.FileInfo) error {
	h := &header{
		Name:     toArchiveName(path),
		Mode:     int64(info.Mode().Perm()),
		Size:     info.Size(),
		ModTime:  info.ModTime().Unix(),
		TypeFlag: typeRegular,
	}
	if err := aw.writeHeader(h); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	// Note: we close explicitly (not just `defer`) so descriptors don't pile
	// up while archiving a large tree — `defer` is function-scoped, so a defer
	// inside this method would only fire after the *whole* archive is built.
	written, err := io.Copy(aw.w, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if written != h.Size {
		return fmt.Errorf("%s: size changed during read (expected %d, wrote %d)", path, h.Size, written)
	}

	// Pad the data out to the next 512-byte boundary with zeros.
	if err := aw.writePadding(h.Size); err != nil {
		return err
	}

	aw.report(h.Name)
	return nil
}

// writeHeader encodes a header struct to its 512-byte block and writes it.
func (aw *archiveWriter) writeHeader(h *header) error {
	blk, err := h.encode()
	if err != nil {
		return err
	}
	_, err = aw.w.Write(blk[:])
	return err
}

// writePadding writes the zero bytes needed to round `size` up to a full block.
// If size is already block-aligned (including size 0) it writes nothing.
func (aw *archiveWriter) writePadding(size int64) error {
	rem := size % blockSize
	if rem == 0 {
		return nil
	}
	pad := make([]byte, blockSize-rem)
	_, err := aw.w.Write(pad)
	return err
}

// finish writes the end-of-archive marker: TWO consecutive all-zero blocks.
// Why two? A single zero block can occur naturally as padding; requiring a pair
// gives an unambiguous "the archive is over" signal that a streaming reader can
// trust without seeking.
func (aw *archiveWriter) finish() error {
	var zero [blockSize * 2]byte
	_, err := aw.w.Write(zero[:])
	return err
}

// report prints the entry name when running with `-v`.
func (aw *archiveWriter) report(name string) {
	if aw.verbose && aw.log != nil {
		fmt.Fprintln(aw.log, name)
	}
}

// toArchiveName normalises a filesystem path into the form stored in the
// archive: forward slashes, and with any leading "./" or "/" stripped so the
// archive holds relative paths (this is also a first line of defence against
// absolute-path extraction surprises).
func toArchiveName(path string) string {
	p := filepath.ToSlash(path)
	for len(p) > 0 && (p[0] == '/' || hasDotSlashPrefix(p)) {
		if p[0] == '/' {
			p = p[1:]
			continue
		}
		p = p[2:] // strip leading "./"
	}
	return p
}

func hasDotSlashPrefix(p string) bool {
	return len(p) >= 2 && p[0] == '.' && p[1] == '/'
}
