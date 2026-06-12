package main

// persistence.go — a tiny snapshot (RDB-style) save/load to disk.
//
// IMPORTANT: this is OUR OWN format. It is intentionally NOT binary-compatible with
// real Redis's RDB files — building the genuine RDB format (with its opcodes,
// length encoding, LZF compression, CRC64 checksum, etc.) would dwarf the rest of
// this challenge. The teaching goal is the *idea* of point-in-time persistence:
// serialise the whole keyspace to a file, then rebuild it on startup.
//
// Format (length-prefixed so values are binary-safe — they may contain spaces,
// CRLF, NUL, anything):
//
//	REDISRDB1\n                          <- magic/version header line
//	<keyLen> <valLen> <expireUnixMilli>\n <- one header line per entry
//	<key bytes><val bytes>                <- the raw bytes, back to back
//	... repeated ...
//
// expireUnixMilli == 0 means "no expiry".
//
// 🐍 For a Python dev: this is hand-rolled `pickle.dump`/`load`. We control every
// byte instead of trusting a serialization library, which is exactly the point.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const rdbMagic = "REDISRDB1"

// Save writes a point-in-time snapshot of the store to path. It writes to a temp
// file first and then renames it over the target, so a crash mid-write can never
// corrupt an existing good snapshot (rename is atomic on the same filesystem).
func Save(store *Store, path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	// bufio.Writer batches many small writes into larger syscalls.
	w := bufio.NewWriter(f)

	if _, err := fmt.Fprintf(w, "%s\n", rdbMagic); err != nil {
		f.Close()
		return err
	}

	for key, e := range store.snapshot() {
		var expireMilli int64
		if !e.expireAt.IsZero() {
			expireMilli = e.expireAt.UnixMilli()
		}
		if _, err := fmt.Fprintf(w, "%d %d %d\n", len(key), len(e.value), expireMilli); err != nil {
			f.Close()
			return err
		}
		if _, err := w.WriteString(key); err != nil {
			f.Close()
			return err
		}
		if _, err := w.WriteString(e.value); err != nil {
			f.Close()
			return err
		}
	}

	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	// fsync so the bytes are durably on disk before we swap the file in.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a snapshot previously written by Save and returns its entries. A
// missing file is NOT an error: a fresh server simply starts empty.
func Load(path string) (map[string]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]entry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("rdb: cannot read header: %w", err)
	}
	if strings.TrimRight(header, "\n") != rdbMagic {
		return nil, fmt.Errorf("rdb: bad magic %q", strings.TrimRight(header, "\n"))
	}

	entries := make(map[string]entry)
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF && line == "" {
			break // clean end of file
		}
		if err != nil && err != io.EOF {
			return nil, err
		}

		fields := strings.Fields(strings.TrimRight(line, "\n"))
		if len(fields) != 3 {
			return nil, fmt.Errorf("rdb: malformed entry header %q", line)
		}
		keyLen, err1 := strconv.Atoi(fields[0])
		valLen, err2 := strconv.Atoi(fields[1])
		expireMilli, err3 := strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || keyLen < 0 || valLen < 0 {
			return nil, fmt.Errorf("rdb: bad entry header %q", line)
		}

		buf := make([]byte, keyLen+valLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("rdb: short read for entry: %w", err)
		}
		key := string(buf[:keyLen])
		val := string(buf[keyLen:])

		e := entry{value: val}
		if expireMilli != 0 {
			e.expireAt = time.UnixMilli(expireMilli)
		}
		entries[key] = e
	}
	return entries, nil
}
