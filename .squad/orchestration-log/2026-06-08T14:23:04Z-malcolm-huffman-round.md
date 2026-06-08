# Orchestration Log: Malcolm (Content Dev) — Phase 1, Challenge 2

**Agent:** Malcolm (Content Dev, claude-opus-4.8)  
**Date:** 2026-06-08T14:23:04Z  
**Requested by:** Himanshu Singh

## Tasks Completed

### Phase 1, Challenge 2: Huffman Compression (Go)

**Path:** `phase-01-foundations/huffman-compression/`

Implemented a lossless file compressor using Huffman coding in Go — the curriculum's first Go code-bearing challenge. Establishes the Go module + internal package skeleton that all ~35 future Go challenges reuse, and factors out `internal/bitio` as a reusable primitive for `tar`, `xxd`, and binary network protocols.

#### Implementation Details

**Module structure:**
- `go.mod` with short module path `module huffman` and toolchain `go 1.22`.
- Thin CLI entry point `main.go` (package `main`): parses `compress`/`decompress` subcommands, `-o` output flag, delegates to libraries, maps errors to exit codes.
- **Internal packages (teaching-sized, one responsibility each):**
  - `internal/bitio/` — General-purpose **BitWriter/BitReader** (MSB-first, zero-padded flush). Huffman-agnostic, fully unit-testable, reusable target for later binary challenges.
  - `internal/huffman/` — Split into `tree.go` (node structure, `CountFrequencies`, greedy build, code table generation), `heap.go` (min-heap via `container/heap`), `codec.go` (`Compress`/`Decompress` API, header format, compression ratio).
- Tests live **beside code** as `*_test.go` files (Go idiom, not a separate `tests/` folder like Python).
- `.gitignore` excludes compiled binary, `*.test`, `*.out`, `*.huf` artifacts — **no build artifacts staged**.

**Header format (HUF1):**
- Magic: `HUF1` (4 bytes).
- `totalSymbols` (uint64 BE) — count of original symbols; enables safe zero-padding in the final byte.
- `numDistinct` (uint16 BE) — distinct byte values in the alphabet.
- Frequency table: `[symbol (1B) + freq (uvarint)]*` — decoder reruns identical Huffman build.
- Bit-packed body: individual symbol codes MSB-first, zero-padded to byte boundary.

**The subtle bug (determinism contract):**
Go randomizes map iteration order. When two frequencies are equal, the tie-break in heap operations must NOT depend on insertion order. **Fix:** Leaf nodes (bytes 0–255) break ties by byte value; internal nodes get stable IDs from 256 upward in creation order. This ensures `Encode` and `Decode` rebuild the *identical* tree despite Go's randomization. The seeded fuzz loop caught this as a round-trip byte mismatch with equal file sizes — fixed before review.

**Edge cases handled:**
- Empty file: header only, no tree, zero symbols.
- Single distinct symbol: depth-0 tree, assign 1-bit code `"0"`, decoder replays that symbol N times.
- All 256 byte values: balanced tree, efficient codes.
- Incompressible data: header cost reported honestly (e.g., single-byte input → 17 B with header; ratio correctly shows loss).

#### Testing

- **Round-trip property:** `Decompress(Compress(x)) == x` (bit-identical) over corpus: empty, single-byte, single-symbol run, plain text, repeated text, all-256 alphabet, random binary, newline-heavy.
- **Fuzz loop:** 50 seeded iterations, caught the map-iteration determinism bug.
- **Bit-I/O units:** 13-bit non-aligned pattern, MSB-first encoding of `0xB2`, EOF, empty flush.
- **Format corruption:** Reject bad magic, truncated body.
- **Prefix-free property:** Assert all codes uniquely decodable.
- **Real file:** 34 KB README.md → 11.7 KB (35.3% saved), sha256 verify round-trip identical.

**Results:** `go vet ./...` clean, `go test ./...` passes.

#### README

All 7 mandated teaching sections:
- 🎯 What We're Building — file compression via Huffman coding.
- 📚 Core Concepts — entropy, Shannon bound, prefix codes, greedy optimality.
- 🏗️ Architecture & Design — frequency table + tree structure, determinism contract (map-iteration pitfall).
- 🔨 Step-by-Step Implementation — tree building, code table, bit I/O, header format.
- 🧪 Testing Strategy — round-trip property, fuzz, edge cases, format rejection.
- 💡 Key Takeaways — greedy principle, information theory, binary format design.
- 📖 Further Reading — references and links.

Includes ASCII diagrams, trade-off table (freq table vs canonical vs tree serialization), and complete run/test instructions.

#### Decisions Recorded

- **Decision: Conventions for Go Code-Bearing Challenges** (`.squad/decisions/decisions.md`) — module layout, internal packages, test placement, CLI contract, `.gitignore`, determinism contract. Applies to all ~35 future Go challenges.

#### CURRICULUM.md Update

Huffman Compression (Challenge 2) checkbox marked ✅ Done.

## Next Steps

Ready for review by Ellie.
