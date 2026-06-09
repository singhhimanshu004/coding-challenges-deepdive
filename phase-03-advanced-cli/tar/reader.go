// reader.go — the LIST (`-t`) and EXTRACT (`-x`) halves of tar. Both walk the
// archive the same way: read a 512-byte header, act on it, then skip past the
// data blocks to the next header. The loop ends at the two-zero-block marker.
//
// 🐍➡️🐹 Python analogy: this mirrors iterating `tarfile.open(mode="r")` and
// calling `.getmembers()` / `.extractall()` — except we decode each record by
// hand and enforce our own safety checks.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archiveReader streams records out of an underlying reader.
type archiveReader struct {
	r io.Reader
}

func newArchiveReader(r io.Reader) *archiveReader {
	return &archiveReader{r: r}
}

// next returns the next header in the archive, or io.EOF when the end-of-archive
// marker (two zero blocks) is reached. The caller is responsible for consuming
// or skipping the entry's data before calling next again.
func (ar *archiveReader) next() (*header, error) {
	var blk [blockSize]byte
	if err := ar.readBlock(&blk); err != nil {
		return nil, err
	}

	h, err := decodeHeader(&blk)
	if errors.Is(err, errZeroBlock) {
		// One zero block seen. The spec requires TWO in a row to end the
		// archive, so confirm the second before declaring EOF.
		if err := ar.readBlock(&blk); err != nil {
			// A truncated/lenient archive that ends after a single zero block
			// still means "done" for our purposes.
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			return nil, err
		}
		if isZeroBlock(&blk) {
			return nil, io.EOF
		}
		// A lone zero block followed by real data is malformed.
		return nil, errors.New("unexpected data after zero block")
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

// readBlock fills exactly one 512-byte block. io.ReadFull turns a short read
// into an error, which is what we want: a real tar block is never partial.
func (ar *archiveReader) readBlock(blk *[blockSize]byte) error {
	_, err := io.ReadFull(ar.r, blk[:])
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// skip advances past `size` bytes of file data PLUS the padding that rounds it
// up to a whole block. Used by list mode, which never reads the data itself.
func (ar *archiveReader) skip(size int64) error {
	padded := roundUp(size)
	_, err := io.CopyN(io.Discard, ar.r, padded)
	return err
}

// roundUp returns size rounded up to the next multiple of blockSize.
func roundUp(size int64) int64 {
	rem := size % blockSize
	if rem == 0 {
		return size
	}
	return size + (blockSize - rem)
}

// ---------------------------------------------------------------------------
// LIST (`-t`)
// ---------------------------------------------------------------------------

// list walks the archive and prints each entry name. With verbose=true it
// prints an `ls -l`-style line (mode, size, mtime, name) the way `tar -tv` does.
func list(r io.Reader, verbose bool, out io.Writer) error {
	ar := newArchiveReader(r)
	for {
		h, err := ar.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		if verbose {
			fmt.Fprintf(out, "%s %8d %s %s\n",
				formatMode(h),
				h.Size,
				time.Unix(h.ModTime, 0).Format("2006-01-02 15:04"),
				h.Name,
			)
		} else {
			fmt.Fprintln(out, h.Name)
		}

		// Listing doesn't touch data — skip straight to the next header.
		if err := ar.skip(h.Size); err != nil {
			return err
		}
	}
}

// formatMode renders a header's type+permissions like `drwxr-xr-x`.
func formatMode(h *header) string {
	var b strings.Builder
	if h.IsDir() {
		b.WriteByte('d')
	} else {
		b.WriteByte('-')
	}
	perm := os.FileMode(h.Mode).Perm()
	b.WriteString(perm.String()[1:]) // drop FileMode's leading type char
	return b.String()
}

// ---------------------------------------------------------------------------
// EXTRACT (`-x`)
// ---------------------------------------------------------------------------

// extract unpacks the archive into destDir, recreating directories and files
// with their stored permissions and modification times. It guards every entry
// against PATH TRAVERSAL so a malicious archive can't write outside destDir.
func extract(r io.Reader, destDir string, verbose bool, log io.Writer) error {
	ar := newArchiveReader(r)
	for {
		h, err := ar.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		// Compute and validate the destination path BEFORE writing anything.
		target, err := safeJoin(destDir, h.Name)
		if err != nil {
			return err
		}

		switch h.TypeFlag {
		case typeDir:
			if err := os.MkdirAll(target, os.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		case typeRegular:
			if err := ar.extractFile(h, target); err != nil {
				return err
			}
		default:
			// Unknown type: skip its data so the stream stays aligned, but
			// don't fail the whole extraction.
			if err := ar.skip(h.Size); err != nil {
				return err
			}
			continue
		}

		// Restore mtime (and, for files, this also confirms perms) after the
		// content is in place. Directory mtimes are set last-write-wins; good
		// enough for our purposes and matches typical tar behaviour.
		_ = os.Chtimes(target, time.Now(), time.Unix(h.ModTime, 0))

		if verbose && log != nil {
			fmt.Fprintln(log, h.Name)
		}
	}
}

// extractFile writes one regular file: create parent dirs, stream exactly
// h.Size bytes from the archive, then consume the block padding so the reader
// is aligned for the next header.
func (ar *archiveReader) extractFile(h *header, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode).Perm())
	if err != nil {
		return err
	}

	// Copy exactly Size bytes — NOT to EOF — because the archive stream
	// continues with the next record right after this file's data+padding.
	_, copyErr := io.CopyN(f, ar.r, h.Size)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	// Explicitly set perms in case the process umask masked bits at create
	// time, so extracted modes match what was archived.
	if err := os.Chmod(target, os.FileMode(h.Mode).Perm()); err != nil {
		return err
	}

	// Discard the padding bytes that follow the data (we've already consumed
	// exactly h.Size data bytes above).
	return ar.skipPadding(h.Size)
}

// skipPadding discards just the zero-padding that follows `size` data bytes,
// i.e. the bytes between the end of the data and the next block boundary.
func (ar *archiveReader) skipPadding(size int64) error {
	pad := roundUp(size) - size
	if pad == 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, ar.r, pad)
	return err
}

// safeJoin builds the on-disk path for an archive entry and refuses anything
// that would escape destDir. This is THE security check for extraction: tar
// archives are untrusted input, and a crafted entry like "../../etc/passwd" or
// an absolute path must never be allowed to write outside the target dir.
func safeJoin(destDir, name string) (string, error) {
	// Reject absolute paths outright.
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("refusing absolute path in archive: %q", name)
	}

	cleanDest, err := filepath.Abs(destDir)
	if err != nil {
		return "", err
	}

	// Join, then Clean collapses any ".." segments. We then verify the result
	// is still inside cleanDest — the only reliable way to catch traversal.
	joined := filepath.Join(cleanDest, filepath.FromSlash(name))
	cleanJoined := filepath.Clean(joined)

	if cleanJoined != cleanDest &&
		!strings.HasPrefix(cleanJoined, cleanDest+string(os.PathSeparator)) {
		return "", fmt.Errorf("refusing path traversal in archive: %q", name)
	}
	return cleanJoined, nil
}
