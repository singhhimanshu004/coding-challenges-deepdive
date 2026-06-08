# Decisions

- Using claude-opus-4.6-1m for all agents per user directive.
- Source material: codingchallenges.fyi — 65+ challenges covering CLI tools, networking, servers, data processing, applications, games, and developer tools.
- Multi-language approach: choose the best language per challenge (Go for CLI/networking, Python for data, TypeScript for web, etc.)
- **README-first learning mandate (user directive):** Every challenge MUST include a comprehensive README.md that explains the concept, how it works in the real world, and a step-by-step walkthrough of the implementation. The goal is actual learning — not just code. Structure: What We're Building → Core Concepts → Architecture → Step-by-Step Implementation → Testing → Key Takeaways → Further Reading.

## Phase 1, Challenge 4: QR Code Generator (Python) — ✅ APPROVED

### Python layout conventions (reaffirmed & reusable)
- Package named after the tool (`qrgen/`), one module per pipeline stage
- `__main__.py` for `python -m <tool>`
- `tests/` package with `pytest.ini` configuration
- `.venv/` and generated artifacts (`*.png`) in `.gitignore`
- `requirements.txt` for dependencies

### Encoder validation methodology
- **Validate with published reference vectors**, not just round-trips
- Reed–Solomon: Wikiversity "Reed–Solomon codes for coders" vector
- QR data codewords: Thonky "HELLO WORLD" V1-Q
- BCH format info: published format-string table
- Reference vectors are decoder-independent and pinpoint failure stages

### Decoder preference for QR
- **Preferred:** `pyzbar` (zbar) — reliably reads small/dense symbols
- **Fallback only:** OpenCV's `QRCodeDetector` — flaky on tiny version-1 symbols
- Make round-trip tests skip cleanly when decoder is unavailable

### macOS native library configuration
- `pip install pyzbar` is insufficient on macOS; also require `brew install zbar`
- Pattern: `tests/conftest.py` prepends common lib dirs to `DYLD_LIBRARY_PATH`/`DYLD_FALLBACK_LIBRARY_PATH`/`LD_LIBRARY_PATH` before decoder import
- Linux: `apt install libzbar0`

### Rendering vs. encoding boundary
- Pillow used for pixel output only — encoding is hand-rolled from scratch
- Keep this distinction clear in challenges with a "build it yourself" mandate
- State explicitly in the README's design section

### Reusable finite-field building blocks
- GF(256) module (log/antilog tables, mul/div/inverse) reusable for future Reed–Solomon/CRC/BCH work
- Shift-register polynomial division applicable to error-correction schemes
- BitBuffer (MSB-first packing) same primitive as Huffman bit-writer (Challenge 2)
