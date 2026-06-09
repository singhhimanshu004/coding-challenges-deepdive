# tar

> **Phase:** 3 — Advanced CLI & Orchestration
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, `bufio`, structs, slices, `io.Reader`/
> `io.Writer`, error returns) back to the Python you already know. This README
> assumes you've skimmed it, and adds 🐍 analogy callouts as we go.

---

## 🎯 What We're Building

`tar` ("**t**ape **ar**chive") bundles a whole directory tree into **one flat
file** — and unpacks it again, byte-for-byte, with permissions and timestamps
intact. It's the `t` in `.tar.gz`: tar does the *packing*, gzip does the
*squeezing*. They're two separate jobs, and here we build the packing half from
scratch.

We implement the four operations you actually use:

| Flag | Meaning | Example |
| ---- | ------- | ------- |
| `-c` | **create** an archive from files/dirs | `tar -cvf out.tar mydir` |
| `-t` | **list** an archive's contents | `tar -tf out.tar` |
| `-x` | **extract** an archive | `tar -xvf out.tar` |
| `-f` | use this **file** (else stdin/stdout) | `-f out.tar` |
| `-v` | **verbose** — print each entry as it's processed | |

Crucially, we implement the **POSIX USTAR on-disk format by hand** — encoding and
decoding the raw 512-byte headers ourselves. We do *not* use Go's `archive/tar`
for the core work. The whole point is to understand the bytes.

> 🐍 In Python you'd reach for `tarfile`. Here we're writing what `tarfile`
> writes, by hand — like implementing `struct.pack`/`unpack` for the tar header
> yourself.

The payoff: by the end, the archive our tool produces can be read by the real
system `tar`, and vice-versa. That interop is the proof we got the format right.

---

## 📚 Core Concepts

### 1. A tar archive is just a grid of 512-byte blocks

There is no central index, no compression, no clever tree. A tar file is a
**linear stream** of fixed-size **512-byte blocks**:

```
┌──────────┬───────────────┬──────────┬───────────────┬─────────────┐
│ header 1 │ file 1 data   │ header 2 │ file 2 data   │  ...  │ 0 0 │
│ (512 B)  │ (padded→512×n)│ (512 B)  │ (padded→512×n)│       │     │
└──────────┴───────────────┴──────────┴───────────────┴─────────────┘
                                                          └─ two zero
                                                             blocks = END
```

Every entry is **one 512-byte header block** describing a file, immediately
followed by that file's **data, padded up to a whole number of 512-byte
blocks**. The archive ends with **two consecutive all-zero blocks**.

Why 512? It's the classic disk/tape sector size. Because *everything* is a
multiple of 512, a reader never has to seek: read 512 bytes → that's a header →
read the octal `size` field → skip that many data bytes (rounded up to 512) →
you're sitting on the next header. **That's the whole algorithm.**

> 🐍 Analogy: it's like a `.jsonl` file where every "line" is exactly 512 bytes
> and strictly typed — you can stream it start to finish without loading the
> whole thing into memory.

### 2. The annotated 512-byte USTAR header

This is the heart of the format. Every field lives at a **fixed byte offset**.
Here is the exact layout we encode/decode in [`header.go`](./header.go):

```
 offset  size  field        notes
 ──────  ────  ───────────  ────────────────────────────────────────────────
   0     100   name         file path, up to 100 bytes, NUL-padded
 100       8   mode         permission bits        ── OCTAL ASCII, e.g. "000644\0"
 108       8   uid          owner user id          ── OCTAL ASCII
 116       8   gid          owner group id         ── OCTAL ASCII
 124      12   size         file size in BYTES     ── OCTAL ASCII (the key field!)
 136      12   mtime        modification time      ── OCTAL ASCII, Unix seconds
 148       8   checksum     header checksum        ── OCTAL ASCII (see §3)
 156       1   typeflag     '0'=file '5'=dir ...
 157     100   linkname     target path for links
 257       6   magic        "ustar\0"  ← identifies the USTAR variant
 263       2   version      "00"
 265      32   uname        owner user name
 297      32   gname        owner group name
 329       8   devmajor     device numbers (device files)
 337       8   devminor
 345     155   prefix       path PREFIX — extends `name` past 100 bytes
 ──────  ────
 500      12   (padding to fill the block out to 512 bytes)
```

Two things surprise everyone the first time:

- **Numbers are stored as ASCII octal text, not binary.** The size `12345`
  bytes is written as the characters `"0000030071\0"` (12345 in octal is
  30071). This is a 1970s portability decision: octal ASCII reads the same on
  every CPU, with no byte-order (endianness) worries. See `writeOctal` /
  `parseOctal`.
- **Long paths split across two fields.** `name` is only 100 bytes. For longer
  paths, USTAR puts the tail in `name` and the head in `prefix` (155 bytes), and
  the reader rejoins them as `prefix + "/" + name`. That's `splitPath` in our
  code, giving an effective limit of 256 bytes.

### 3. The checksum (the integrity check)

Bytes 148–155 hold a **checksum** of the whole header. The algorithm is
delightfully simple, with one twist:

> **Sum all 512 bytes of the header as unsigned values — but while summing,
> pretend the 8 checksum bytes are ASCII spaces (`0x20`).**

The "pretend it's spaces" rule exists because the checksum can't include itself.
So the spec freezes those 8 bytes to a known value (spaces) during the
computation. We then store the result as 6 octal digits + `NUL` + space.

On read, we recompute the sum the same way and compare. A mismatch means the
data isn't a valid header — corruption, or simply not a tar file. This is the
first line of defence in `decodeHeader`. Getting the spaces rule wrong is the
single most common reason a hand-written tar is rejected by `/usr/bin/tar`.

### 4. Block padding & the two-zero-block terminator

A 600-byte file occupies **two** data blocks: 512 full + 88 used + 424 zero
padding = 1024 bytes. Padding keeps the next header on a 512-byte boundary.

The archive ends with **two** all-zero blocks. Why two, not one? A single zero
block can show up naturally (it's valid padding), so one zero block is
ambiguous. **Two in a row** is the unmistakable "stream is over" signal — which
matters precisely *because* tar is a streaming format with no length header up
front.

### 5. Why streaming archive formats matter

tar was built for **tape drives** — sequential media you can only read
front-to-back. That constraint produced a format that is:

- **Append-friendly / pipeline-friendly:** you can generate it on the fly and
  pipe it straight into another process (`tar -c dir | gzip | ssh host 'tar -x'`)
  without ever knowing the total size in advance.
- **Memory-flat:** create and extract touch one block at a time; a 100 GB
  archive needs only a 512-byte buffer.
- **Composable:** because tar only *packs* (no compression), you bolt on gzip,
  bzip2, or zstd separately. One job, done well — the Unix philosophy in a file
  format.

That's why, decades after tapes, tar is still the backbone of `.tar.gz`
releases, Docker image layers, and `kubectl cp`.

---

## 🏗️ Architecture & Design

A clean split mirrors the three jobs, one file each:

```
header.go   ── encode/decode of the 512-byte USTAR block (the format itself)
writer.go   ── CREATE (-c): walk the tree, emit header+data+padding, terminate
reader.go   ── LIST (-t) & EXTRACT (-x): read header, act, skip to next
main.go     ── CLI: parse -c/-x/-t/-f/-v, wire streams, dispatch
tar_test.go ── round-trips, checksum, full create→list→extract, traversal guard
```

Design choices, consistent with the other Go tools in this repo:

- **`header` struct as the boundary.** Raw byte offsets live *only* in
  `header.go`. The reader produces `header` structs; the writer consumes them.
  Nothing else pokes at offset 124. 🐍 Like a dataclass that hides its
  `struct.pack` format string.
- **`io.Reader` / `io.Writer` everywhere.** `archiveWriter` writes to *any*
  `io.Writer` and `archiveReader` reads from *any* `io.Reader`, so tests use an
  in-memory `bytes.Buffer` with **no temp files and no subprocess**. 🐍 This is
  Go's version of duck typing — "anything with a `.write()`" — but checked at
  compile time.
- **Thin `main`.** `main()` just calls `run(args, stdout, stderr) int`, our
  settled convention across every Go challenge, so the CLI is unit-testable.
- **Exit codes:** `0` success, `1` domain/IO failure, `2` usage error.

---

## 🔨 Step-by-Step Implementation

1. **Field codecs first** (`writeOctal`/`parseOctal`, `writeString`/
   `parseString`). Everything else is built on reading/writing octal-ASCII and
   NUL-padded text at fixed offsets. Unit-test these with a round-trip.
2. **`header.encode()` / `decodeHeader()`.** Lay out the block, then compute the
   checksum **last** (it sums the finished block). On decode, **verify the
   checksum first** before trusting any field.
3. **`splitPath`** so paths > 100 bytes use the `prefix` field. Round-trip a
   long path to prove it.
4. **Writer (`-c`).** `addPath` → recurse dirs (header-then-children, sorted for
   determinism), `addFile` writes header + `io.Copy` of data + padding. Finish
   with two zero blocks.
5. **Reader (`-t`/`-x`).** `next()` reads a block, decodes it, and treats a zero
   block as a possible terminator (confirm the second). List skips data; extract
   recreates dirs/files and restores mode + mtime.
6. **Safety.** `safeJoin` rejects absolute paths and `..` traversal *before*
   writing. Extraction is untrusted input — this check is non-negotiable.
7. **CLI.** Hand-rolled flag parser accepts bundled flags (`-cvf out.tar`) just
   like real tar.

### The biggest Go gotchas flagged in the code

- **`defer f.Close()` is function-scoped, not block-scoped.** Deferring a close
  inside a loop that archives thousands of files leaks descriptors until the
  whole function returns. We `Close()` explicitly per file. (Coming from
  Python's `with`, this is the #1 mental-model trap.)
- **Strings index by *byte*, not character.** Header fields are byte ranges; we
  slice `[]byte`, never assume `len(s)` is a character count.
- **`io.CopyN(dst, src, size)` — not `io.Copy`!** On extract we must copy
  *exactly* `size` bytes, because the archive stream continues with the next
  record right after the data. Copying to EOF would swallow the rest of the
  archive.
- **`io.EOF` is a value, not a crash.** `next()` returns it to mean "clean end".

---

## 🧪 Testing Strategy

`go test ./...` covers (see [`tar_test.go`](./tar_test.go)):

- **Header round-trip** — encode → decode returns identical fields, including a
  > 100-byte path that exercises the `prefix`/`name` split.
- **Checksum** — corrupting one header byte makes `decodeHeader` reject it.
- **Full create → list → extract** on a real temp-dir tree with subdirectories
  and an empty directory, asserting **content, file mode, and mtime
  preservation**.
- **Block padding** — a 600-byte file produces exactly two data blocks (1024).
- **Path-traversal rejection** — a hand-built archive with a `../escape.txt`
  entry is refused, and nothing is written outside the destination.
- **Empty archive** — `finish()` alone is exactly two zero blocks and lists as
  nothing.

`go vet ./...` is clean.

### Interop with the real `tar` (the real proof)

The strongest correctness signal is that the system tool agrees with ours, both
directions:

```bash
go build -o ./tar .

# 1. We create, the system lists & extracts it
./tar -cvf ours.tar tree
tar -tf ours.tar                              # system tar reads our archive
mkdir out && cd out && tar -xf ../ours.tar    # system tar extracts ours

# 2. The system creates, we list & extract it
tar -cf sys.tar tree
./tar -tf sys.tar
./tar -xvf sys.tar                            # we extract a system-made tar
```

Both directions were verified: listings match, file contents match, and a
`640` permission survived our-create → system-extract.

> ⚠️ macOS's system `tar` is **bsdtar**, which can write a richer "pax" variant
> by default (extended headers). Our reader handles plain USTAR records and
> skips unknown entry types gracefully; our *output* is pure USTAR, which both
> bsdtar and GNU tar read happily.

---

## 💡 Key Takeaways

- **A tar file is a stream of self-describing 512-byte blocks** — header, then
  padded data, then two zero blocks. No index, no seeking. That single idea is
  the entire format.
- **Numbers are octal ASCII text** for byte-order-free portability; this is the
  same "encode binary as a fixed-width field" muscle from Huffman's bit I/O in
  Phase 1, one level up.
- **The checksum's "treat my own field as spaces" trick** is the classic
  self-reference fix, and the usual reason a hand-rolled tar fails to interop.
- **Extraction is untrusted input** — validating against path traversal and
  absolute paths is a security requirement, not a nicety.
- **`io.Reader`/`io.Writer` make systems code testable in memory** — the same
  injectable-streams pattern we use across every Go tool here.

---

## 📖 Further Reading

- [GNU tar manual — Standard (USTAR) format](https://www.gnu.org/software/tar/manual/html_node/Standard.html)
- [POSIX `pax`/`ustar` header spec (Open Group)](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06)
- [Wikipedia — tar (computing)](https://en.wikipedia.org/wiki/Tar_(computing))
- Go's [`archive/tar`](https://pkg.go.dev/archive/tar) — read it *after* this, to
  compare the standard library's take with our from-scratch version.
- [`codingchallenges.fyi` — Build Your Own tar](https://codingchallenges.fyi/challenges/challenge-tar)
