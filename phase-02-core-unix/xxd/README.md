# xxd

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🆕 **New to Go?** Read the project's [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) first — it maps the Go idioms used here (interfaces, `defer`, slices, multiple returns) onto things you already know from Python.

---

## 🎯 What We're Building

`xxd` makes a **hex dump**: a human-readable view of the *raw bytes* in a file. Open a `.png`, a compiled binary, or a captured network packet in a text editor and you get mojibake — garbage — because those bytes aren't text. `xxd` translates each byte into its two-digit hexadecimal value and lines them up in neat columns so you can actually *read* binary data.

It also runs **in reverse** (`-r`): give it a hex dump and it reconstructs the original bytes. That round-trip — bytes → text → bytes — is how you safely paste binary data into an email, a chat message, or a patch file and get it back intact on the other end.

```
$ printf 'hello\n' | xxd
00000000: 6865 6c6c 6f0a                           hello.

$ printf 'hello\n' | xxd | xxd -r
hello
```

This is the **byte-level mindset** you'll lean on for the rest of the curriculum: `tar` archives, DNS packets, NTP timestamps, and TLS records are all just bytes in a defined layout. `xxd` is the microscope you use to inspect them.

## 📚 Core Concepts

### 1. Why hexadecimal?

A byte is 8 bits — a value from 0 to 255. Hexadecimal (base 16) is the natural way to write it because **two hex digits = exactly one byte** (`0x00` … `0xff`). The mapping is rigid and lossless, which is what you want when every bit matters. Decimal would need a variable 1–3 digits per byte and wouldn't align into columns; binary would be 8 digits per byte and far too wide.

| Byte (char) | Decimal | Hex  |
|-------------|---------|------|
| `h`         | 104     | `68` |
| `\n` (newline) | 10   | `0a` |
| `\0` (NUL)  | 0       | `00` |
| `ÿ`         | 255     | `ff` |

### 2. Anatomy of a dump line

Every line has three regions. Here is one annotated with a default `-c 16 -g 2` layout:

```
00000000: 6865 6c6c 6f20 776f 726c 640a 7365 636f  hello world.seco
└───┬───┘ └──────────────────┬───────────────────┘  └──────┬──────┘
  offset            hex byte columns                   ASCII gutter
 (address)      (grouped 2 bytes at a time)        (printable bytes only)
```

- **Offset column** — an 8-digit hex *address* telling you how far into the file this line starts. Line 1 is `00000000`, and with the default 16 bytes per line, line 2 is `00000010` (decimal 16). This is your map: "the bug is at offset 0x2f."
- **Hex columns** — the bytes themselves. They're **grouped** (default: 2 bytes per group, separated by a single space) purely for readability — the grouping has no effect on the data.
- **ASCII gutter** — the same bytes rendered as text *if* they're printable (`0x20`–`0x7e`). Anything else — NULs, tabs, high bytes — shows as `.`. This lets your eye pick out strings buried inside binary data.

### 3. Printable vs. non-printable

The gutter only prints bytes in the range `0x20` (space) through `0x7e` (`~`). Everything else becomes `.`. That's why a newline (`0a`) and a NUL (`00`) both look like a dot — the gutter is a *lossy* convenience view. The **hex columns are the source of truth**; the gutter is just there to help humans spot text.

### 4. The reverse round-trip (`-r`)

Because the hex columns losslessly describe every byte, the dump can be parsed back into the original. The parser:

1. reads the **offset** before the `:` (used to reproduce any gaps),
2. reads the **hex columns**, and
3. **ignores the ASCII gutter** entirely — the bytes are already fully described by the hex.

The subtle trick is knowing where the hex ends and the gutter begins. The separator is a run of **two spaces**, while the gaps *inside* the hex area are always single spaces. So "the first double space after the hex" marks the boundary. (See `parseHexColumns` in `reverse.go`.)

### 5. Binary-safe I/O

`xxd` must never treat its input as text — a byte is a byte. On the forward path we read with `io.ReadFull` into a byte buffer and never decode runes, so embedded NULs and arbitrary high bytes pass through untouched. The round-trip test proves this by dumping and reversing all 256 possible byte values.

## 🏗️ Architecture & Design

The code is a clean split between the two directions, plus a thin CLI:

| File | Responsibility |
|------|----------------|
| `main.go` | CLI entry point, hand-rolled flag parsing, input selection (file or stdin). |
| `dump.go` | **Forward engine** — bytes → hex dump (`dump`, `writeLine`). |
| `reverse.go` | **Reverse engine** — hex dump → bytes (`reverse`, `parseHexColumns`). |
| `xxd_test.go` | Tests for every flag, the reverse path, and the binary round-trip. |

The core functions take `io.Reader` / `io.Writer` interfaces rather than concrete files. **Go idiom:** this is duck typing made explicit — a real file, `os.Stdin`, or an in-memory `bytes.Buffer` all satisfy those interfaces, which is exactly what lets the tests feed in strings instead of opening files.

### Flags

| Flag | Meaning | Default |
|------|---------|---------|
| `-l len` | dump at most `len` bytes | unlimited |
| `-c cols` | bytes per output line | 16 |
| `-s seek` | skip `seek` bytes before dumping (offset column starts there) | 0 |
| `-g group` | bytes per space-separated group (`0` = one big group) | 2 |
| `-r` | **reverse**: parse a dump back into bytes | off |

With no file operand (or `-`), `xxd` reads standard input.

### Exit codes (repo convention)

| Code | Meaning |
|------|---------|
| `0` | success |
| `1` | domain failure (file open / read / parse error) |
| `2` | usage error (bad flag) |

## 🔨 Step-by-Step Implementation

1. **Parse flags** (`parseArgs`). Hand-rolled so both attached (`-c16`) and separated (`-c 16`) forms work, exactly like the real tool — Go's standard `flag` package can't do attached short flags.
2. **Skip with `-s`** (`io.CopyN(io.Discard, …)`). This works even on a pipe where seeking is illegal, keeping us stdin-safe.
3. **Read a line's worth of bytes** with `io.ReadFull`, clamped by the remaining `-l` budget.
4. **Format the line** (`writeLine`): print the offset, walk all `cols` slots (padding missing ones with spaces so short final lines stay aligned), then the ASCII gutter.
5. **Reverse** (`reverse`): scan line by line, split off the offset, read hex up to the double-space gutter, decode nibble pairs into bytes, and pad zero bytes for any address gap.

## 🧪 Testing Strategy

Run the suite:

```bash
go test ./...   # all unit tests + round-trip
go vet ./...    # static checks
```

The tests cover:

- the **default** dump and its short-line padding,
- each flag: `-c`, `-g` (including `-g 1` and `-g 4`), `-l`, `-s`,
- **non-printable** bytes rendering as `.`,
- **empty input** (produces no output),
- the **reverse** path on a known dump, and
- a **DUMP → REVERSE → original** round-trip across all 256 byte values under several `-c`/`-g` configs — the binary-safety guarantee.
- the full `cli()` entry point over **stdin**, including a reverse round-trip.

### Verified against the real tool

Forward output is **byte-for-byte identical** to system `xxd`. To confirm:

```bash
go build -o xxd .
printf 'hello\x00\x01world\tand more' | diff <(xxd) <(./xxd) && echo "forward OK"
printf 'hello\x00\x01world\tand more' | ./xxd | ./xxd -r | cmp - <(printf 'hello\x00\x01world\tand more') && echo "round-trip OK"
```

## 💡 Key Takeaways

- **Two hex digits = one byte.** Hex is the lingua franca of binary because the mapping is rigid and lossless.
- **The hex columns are the truth; the ASCII gutter is a lossy convenience.** Two different bytes (`0a` and `00`) can both show as `.`.
- **Fixed-width formatting is deliberate.** Padding short lines keeps the gutter aligned and makes a dump easy to scan — and easy to parse back.
- **Lossless round-trips need an unambiguous delimiter.** The "single space inside hex, double space before the gutter" rule is what lets `-r` separate data from decoration.
- **Take interfaces, not files.** Writing `dump`/`reverse` against `io.Reader`/`io.Writer` made them trivially testable with in-memory buffers — a pattern you'll reuse in every Go challenge.
- This is the on-ramp to **binary protocols**: once you can read a hex dump, a DNS packet or a TLS record stops being scary and becomes just bytes in a layout.

## 📖 Further Reading

- Challenge spec — [Build Your Own xxd (codingchallenges.fyi)](https://codingchallenges.fyi/challenges/challenge-xxd/)
- `man xxd` — the reference implementation's manual
- Project primer — [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
- Related challenges — Huffman (Phase 1, bit-level I/O), `tar` (binary archive format), and the Phase 4 network protocols (DNS, NTP) that build on this byte-level fluency.
