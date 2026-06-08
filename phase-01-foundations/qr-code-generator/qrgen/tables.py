"""Static QR-code specification data for versions 1–10.

Everything here comes straight from the ISO/IEC 18004 standard. Pulling it into
one module keeps the algorithmic code (encoding, matrix, masking) readable.

Key concepts encoded here:
    * A QR "version" V (1..40) is a size: the symbol is (17 + 4*V) modules
      square. V1 = 21x21, V2 = 25x25, ... V10 = 57x57. We support 1..10.
    * Each (version, EC level) pairs a fixed number of *data* codewords with
      *error-correction* codewords, and splits them into one or more blocks so a
      burst of damage can't wipe out a whole block. ECC_TABLE captures that.
    * Alignment patterns appear at fixed coordinates that grow with version.
"""

from __future__ import annotations

# Error-correction levels, ordered weakest→strongest (more EC = less data).
EC_LEVELS = ("L", "M", "Q", "H")

# Mode indicators (4-bit prefixes that tell the decoder how the payload is
# encoded). We implement byte mode fully; numeric/alphanumeric are listed for
# reference and used by the analyzer when the input qualifies.
MODE_NUMERIC = 0b0001
MODE_ALPHANUMERIC = 0b0010
MODE_BYTE = 0b0100

# The 45-character alphabet for alphanumeric mode (indices are the code values).
ALPHANUMERIC_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

# ECC_TABLE[version][ec_level] = (ec_codewords_per_block, blocks)
# where `blocks` is a list of (num_blocks, data_codewords_per_block) groups.
# total data codewords = sum(num * data); total codewords = data + ec*num_blocks.
ECC_TABLE: dict[int, dict[str, tuple[int, list[tuple[int, int]]]]] = {
    1: {
        "L": (7, [(1, 19)]),
        "M": (10, [(1, 16)]),
        "Q": (13, [(1, 13)]),
        "H": (17, [(1, 9)]),
    },
    2: {
        "L": (10, [(1, 34)]),
        "M": (16, [(1, 28)]),
        "Q": (22, [(1, 22)]),
        "H": (28, [(1, 16)]),
    },
    3: {
        "L": (15, [(1, 55)]),
        "M": (26, [(1, 44)]),
        "Q": (18, [(2, 17)]),
        "H": (22, [(2, 13)]),
    },
    4: {
        "L": (20, [(1, 80)]),
        "M": (18, [(2, 32)]),
        "Q": (26, [(2, 24)]),
        "H": (16, [(4, 9)]),
    },
    5: {
        "L": (26, [(1, 108)]),
        "M": (24, [(2, 43)]),
        "Q": (18, [(2, 15), (2, 16)]),
        "H": (22, [(2, 11), (2, 12)]),
    },
    6: {
        "L": (18, [(2, 68)]),
        "M": (16, [(4, 27)]),
        "Q": (24, [(4, 19)]),
        "H": (28, [(4, 15)]),
    },
    7: {
        "L": (20, [(2, 78)]),
        "M": (18, [(4, 31)]),
        "Q": (18, [(2, 14), (4, 15)]),
        "H": (26, [(4, 13), (1, 14)]),
    },
    8: {
        "L": (24, [(2, 97)]),
        "M": (22, [(2, 38), (2, 39)]),
        "Q": (22, [(4, 18), (2, 19)]),
        "H": (26, [(4, 14), (2, 15)]),
    },
    9: {
        "L": (30, [(2, 116)]),
        "M": (22, [(3, 36), (2, 37)]),
        "Q": (20, [(4, 16), (4, 17)]),
        "H": (24, [(4, 12), (4, 13)]),
    },
    10: {
        "L": (18, [(2, 68), (2, 69)]),
        "M": (26, [(4, 43), (1, 44)]),
        "Q": (24, [(6, 19), (2, 20)]),
        "H": (28, [(6, 15), (2, 16)]),
    },
}

# Center coordinates of alignment patterns per version. The Cartesian product of
# a list with itself gives every alignment-pattern center; combinations that
# collide with the three finder patterns are skipped during matrix layout.
ALIGNMENT_POSITIONS: dict[int, list[int]] = {
    1: [],
    2: [6, 18],
    3: [6, 22],
    4: [6, 26],
    5: [6, 30],
    6: [6, 34],
    7: [6, 22, 38],
    8: [6, 24, 42],
    9: [6, 26, 46],
    10: [6, 28, 50],
}

MAX_VERSION = 10


def modules_for_version(version: int) -> int:
    """Side length (in modules) of the symbol for a given version."""
    return 17 + 4 * version


def total_data_codewords(version: int, ec_level: str) -> int:
    """Number of usable data codewords (bytes) for a (version, EC level)."""
    _, blocks = ECC_TABLE[version][ec_level]
    return sum(num * data for num, data in blocks)


def char_count_bits(version: int, mode: int) -> int:
    """Width of the character-count indicator, which grows with version."""
    if mode == MODE_BYTE:
        return 8 if version <= 9 else 16
    if mode == MODE_NUMERIC:
        return 10 if version <= 9 else 12
    if mode == MODE_ALPHANUMERIC:
        return 9 if version <= 9 else 11
    raise ValueError(f"unsupported mode {mode}")
