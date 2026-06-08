"""Matrix construction — painting codewords into the 2D QR symbol.

A QR symbol is a grid of dark/light *modules*. Some modules are fixed
"function patterns" that orient the reader; the rest carry data. This module:

    1. Lays down the function patterns (finders, separators, timing, alignment,
       the lone dark module) and reserves the format/version-info strips.
    2. Walks the remaining cells in the standard zig-zag and drops the encoded
       codeword bits into them.
    3. After a mask is chosen (see mask.py) writes the 15-bit format info and,
       for versions ≥ 7, the 18-bit version info — both protected by BCH codes.

ASCII sketch of the V1 (21×21) function patterns:

    ███████ . ▒ . ███████      █ finder pattern (7×7)
    █     █ . ▒ . █     █       ▒ timing pattern (alternating)
    █ ███ █ . ▒ . █ ███ █       . separator (light)
    █ ███ █ . ▒ . █ ███ █
    █ ███ █ . ▒ . █ ███ █
    █     █ . ▒ . █     █
    ███████ . █ . ███████      <- dark module sits just above bottom-left finder
    . . . . . . . . . . .
    ▒ ▒ ▒ ▒ ▒ . ▒ . . . .      <- timing patterns bridge the finders
"""

from __future__ import annotations

from . import tables

# A module's value is 0 (light) or 1 (dark); None means "not yet written".
DARK = 1
LIGHT = 0

# BCH parameters for the 15-bit format information code.
_FORMAT_GENERATOR = 0b101_0011_0111  # 0x537
_FORMAT_MASK = 0b101_0100_0001_0010  # 0x5412, XOR'd in so all-zero is invalid
# EC level → 2-bit field used inside format info (note: not L<M<Q<H order!).
_EC_FORMAT_BITS = {"L": 0b01, "M": 0b00, "Q": 0b11, "H": 0b10}

# BCH parameter for the 18-bit version information code (versions ≥ 7).
_VERSION_GENERATOR = 0b1_1111_0010_0101  # 0x1F25


class QRMatrix:
    """The module grid plus a parallel map of which cells are function cells."""

    def __init__(self, version: int) -> None:
        self.version = version
        self.size = tables.modules_for_version(version)
        self.modules: list[list[int | None]] = [
            [None] * self.size for _ in range(self.size)
        ]
        # reserved[r][c] is True for function patterns and format/version strips,
        # which the data-placement walk must skip.
        self.reserved: list[list[bool]] = [
            [False] * self.size for _ in range(self.size)
        ]

    # --- function patterns -------------------------------------------------

    def _set(self, r: int, c: int, value: int, reserved: bool = True) -> None:
        self.modules[r][c] = value
        self.reserved[r][c] = reserved

    def _place_finder(self, top: int, left: int) -> None:
        """Draw one 7×7 finder pattern (concentric dark square)."""
        for r in range(-1, 8):
            for c in range(-1, 8):
                rr, cc = top + r, left + c
                if not (0 <= rr < self.size and 0 <= cc < self.size):
                    continue
                # The 1-module ring around the finder is the light separator.
                if r in (-1, 7) or c in (-1, 7):
                    self._set(rr, cc, LIGHT)
                elif r in (0, 6) or c in (0, 6):
                    self._set(rr, cc, DARK)
                elif 2 <= r <= 4 and 2 <= c <= 4:
                    self._set(rr, cc, DARK)
                else:
                    self._set(rr, cc, LIGHT)

    def _place_timing(self) -> None:
        """The two timing lines: alternating dark/light at row/col 6."""
        for i in range(8, self.size - 8):
            value = DARK if i % 2 == 0 else LIGHT
            if self.modules[6][i] is None:
                self._set(6, i, value)
            if self.modules[i][6] is None:
                self._set(i, 6, value)

    def _place_alignment(self) -> None:
        """5×5 alignment patterns at every valid center for this version."""
        centers = tables.ALIGNMENT_POSITIONS[self.version]
        for r in centers:
            for c in centers:
                # Skip centers overlapping the three finder patterns.
                if (r, c) in ((6, 6), (6, self.size - 7), (self.size - 7, 6)):
                    continue
                for dr in range(-2, 3):
                    for dc in range(-2, 3):
                        ring = max(abs(dr), abs(dc))
                        value = DARK if ring != 1 else LIGHT
                        self._set(r + dr, c + dc, value)

    def _reserve_format_areas(self) -> None:
        """Reserve (but don't yet fill) the format and version info strips."""
        size = self.size
        for i in range(9):
            if self.modules[8][i] is None:
                self._set(8, i, LIGHT)
            if self.modules[i][8] is None:
                self._set(i, 8, LIGHT)
        for i in range(8):
            self._set(8, size - 1 - i, LIGHT)
            self._set(size - 1 - i, 8, LIGHT)
        # The dark module — always dark, always at this fixed spot.
        self._set(size - 8, 8, DARK)
        if self.version >= 7:
            for i in range(6):
                for j in range(3):
                    self._set(size - 11 + j, i, LIGHT)
                    self._set(i, size - 11 + j, LIGHT)

    def build_function_patterns(self) -> None:
        """Lay down everything that isn't data, in dependency order."""
        self._place_finder(0, 0)
        self._place_finder(0, self.size - 7)
        self._place_finder(self.size - 7, 0)
        self._place_alignment()
        self._place_timing()
        self._reserve_format_areas()

    # --- data placement ----------------------------------------------------

    def place_data(self, codewords: list[int]) -> None:
        """Walk the zig-zag and drop each codeword bit into a free cell.

        The walk moves in 2-module-wide vertical columns, bottom→top then
        top→bottom, skipping the vertical timing column at x=6.
        """
        bits = [(byte >> i) & 1 for byte in codewords for i in range(7, -1, -1)]
        idx = 0
        size = self.size
        col = size - 1
        upward = True
        while col > 0:
            if col == 6:  # skip the vertical timing pattern column
                col -= 1
            rows = range(size - 1, -1, -1) if upward else range(size)
            for row in rows:
                for c in (col, col - 1):
                    if self.modules[row][c] is None:
                        self.modules[row][c] = bits[idx] if idx < len(bits) else 0
                        idx += 1
            col -= 2
            upward = not upward

    # --- format & version information --------------------------------------

    def _format_bits(self, ec_level: str, mask: int) -> int:
        """Compute the 15-bit BCH-protected format information value."""
        data = (_EC_FORMAT_BITS[ec_level] << 3) | mask
        # Standard BCH(15,5): divide data*2^10 by the generator, keep remainder.
        rem = data << 10
        while rem.bit_length() - 1 >= 10:
            rem ^= _FORMAT_GENERATOR << (rem.bit_length() - 11)
        return ((data << 10) | rem) ^ _FORMAT_MASK

    def write_format_info(self, ec_level: str, mask: int) -> None:
        """Stamp the 15-bit format info into both redundant locations."""
        bits = self._format_bits(ec_level, mask)
        size = self.size
        # Sequence of 15 bits, MSB first.
        seq = [(bits >> (14 - i)) & 1 for i in range(15)]

        # Copy 1: around the top-left finder.
        coords1 = []
        for i in range(6):
            coords1.append((8, i))
        coords1.append((8, 7))
        coords1.append((8, 8))
        coords1.append((7, 8))
        for i in range(6):
            coords1.append((5 - i, 8))
        for (r, c), b in zip(coords1, seq):
            self.modules[r][c] = b
            self.reserved[r][c] = True

        # Copy 2: split under the top-right and beside the bottom-left finders.
        coords2 = []
        for i in range(8):
            coords2.append((size - 1 - i, 8))
        for i in range(8):
            coords2.append((8, size - 8 + i))
        for (r, c), b in zip(coords2, seq):
            self.modules[r][c] = b
            self.reserved[r][c] = True

    def write_version_info(self) -> None:
        """Stamp the 18-bit version info (only present for versions ≥ 7)."""
        if self.version < 7:
            return
        rem = self.version << 12
        while rem.bit_length() - 1 >= 12:
            rem ^= _VERSION_GENERATOR << (rem.bit_length() - 13)
        bits = (self.version << 12) | rem
        seq = [(bits >> i) & 1 for i in range(18)]
        size = self.size
        for i in range(18):
            r, c = i // 3, i % 3
            self.modules[size - 11 + c][r] = seq[i]
            self.reserved[size - 11 + c][r] = True
            self.modules[r][size - 11 + c] = seq[i]
            self.reserved[r][size - 11 + c] = True
