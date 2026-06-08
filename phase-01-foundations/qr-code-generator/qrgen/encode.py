"""Data analysis and encoding — turning text into the final QR bitstream.

This module owns the front half of the QR pipeline:

    text  ->  mode analysis  ->  smallest fitting version  ->  bit encoding
          ->  terminator + padding  ->  split into blocks
          ->  Reed–Solomon ECC per block  ->  interleave  ->  codeword stream

The output is the exact sequence of bytes (codewords) that the matrix module
will paint into the symbol, in the order the standard requires.
"""

from __future__ import annotations

from dataclasses import dataclass

from . import reedsolomon, tables


class BitBuffer:
    """A growable MSB-first bit accumulator.

    QR packs fields of arbitrary bit-width (4-bit mode, 8/16-bit counts, 10-bit
    numeric triplets...) into a continuous stream, then slices it back into
    8-bit codewords. A simple list of 0/1 ints keeps that logic obvious.
    """

    def __init__(self) -> None:
        self.bits: list[int] = []

    def append_bits(self, value: int, length: int) -> None:
        """Append the low `length` bits of `value`, most-significant first."""
        for i in range(length - 1, -1, -1):
            self.bits.append((value >> i) & 1)

    def __len__(self) -> int:
        return len(self.bits)

    def to_codewords(self) -> list[int]:
        """Pack the bit list into 8-bit codewords (assumes byte-aligned)."""
        return [
            int("".join(str(b) for b in self.bits[i:i + 8]), 2)
            for i in range(0, len(self.bits), 8)
        ]


@dataclass
class EncodedData:
    """Everything the matrix builder needs about the encoded payload."""

    version: int
    ec_level: str
    mode: int
    codewords: list[int]  # interleaved data + EC codewords, in placement order


def analyze_mode(text: str) -> int:
    """Pick the most compact encoding mode the input qualifies for.

    Numeric ⊂ Alphanumeric ⊂ Byte in terms of what they can represent, but the
    more restrictive modes pack more characters per bit, so we prefer them.
    """
    if all(c in "0123456789" for c in text):
        return tables.MODE_NUMERIC
    if all(c in tables.ALPHANUMERIC_CHARS for c in text):
        return tables.MODE_ALPHANUMERIC
    return tables.MODE_BYTE


def _payload_bit_length(data: bytes, mode: int, char_count: int) -> int:
    """Bit length of just the encoded payload (excludes header/count)."""
    if mode == tables.MODE_BYTE:
        return 8 * len(data)
    if mode == tables.MODE_NUMERIC:
        full, rem = divmod(char_count, 3)
        return full * 10 + (7 if rem == 2 else 4 if rem == 1 else 0)
    if mode == tables.MODE_ALPHANUMERIC:
        full, rem = divmod(char_count, 2)
        return full * 11 + (6 if rem == 1 else 0)
    raise ValueError(f"unsupported mode {mode}")


def choose_version(data: bytes, text: str, mode: int, ec_level: str) -> int:
    """Smallest version (1..10) whose data capacity holds this payload."""
    char_count = len(text) if mode != tables.MODE_BYTE else len(data)
    for version in range(1, tables.MAX_VERSION + 1):
        capacity_bits = tables.total_data_codewords(version, ec_level) * 8
        header = 4 + tables.char_count_bits(version, mode)
        needed = header + _payload_bit_length(data, mode, char_count)
        if needed <= capacity_bits:
            return version
    raise ValueError(
        "input too large for supported versions (1–10); "
        f"need more than {tables.MAX_VERSION} or a lower EC level"
    )


def _encode_payload(buf: BitBuffer, data: bytes, text: str, mode: int) -> None:
    """Append the mode-specific payload bits to the buffer."""
    if mode == tables.MODE_BYTE:
        for byte in data:
            buf.append_bits(byte, 8)
    elif mode == tables.MODE_NUMERIC:
        for i in range(0, len(text), 3):
            chunk = text[i:i + 3]
            buf.append_bits(int(chunk), {1: 4, 2: 7, 3: 10}[len(chunk)])
    elif mode == tables.MODE_ALPHANUMERIC:
        a = tables.ALPHANUMERIC_CHARS
        for i in range(0, len(text), 2):
            if i + 1 < len(text):
                buf.append_bits(a.index(text[i]) * 45 + a.index(text[i + 1]), 11)
            else:
                buf.append_bits(a.index(text[i]), 6)
    else:
        raise ValueError(f"unsupported mode {mode}")


def _add_terminator_and_padding(buf: BitBuffer, total_codewords: int) -> None:
    """Close out the stream: terminator, byte-align, then alternating pad bytes."""
    capacity = total_codewords * 8
    # Up to four 0-bits signal "end of data".
    terminator = min(4, capacity - len(buf))
    buf.append_bits(0, terminator)
    # Pad with 0s to the next byte boundary.
    if len(buf) % 8 != 0:
        buf.append_bits(0, 8 - len(buf) % 8)
    # Fill remaining codewords with the fixed alternating pad bytes 0xEC, 0x11.
    pad_bytes = (0xEC, 0x11)
    i = 0
    while len(buf) < capacity:
        buf.append_bits(pad_bytes[i % 2], 8)
        i += 1


def _structure_codewords(
    data_codewords: list[int], version: int, ec_level: str
) -> list[int]:
    """Split into blocks, compute RS ECC, and interleave per the standard.

    QR interleaves codewords across blocks so a localized smudge spreads its
    damage thinly across every block (each of which can self-heal a little),
    rather than destroying one block entirely.
    """
    ec_per_block, groups = tables.ECC_TABLE[version][ec_level]

    data_blocks: list[list[int]] = []
    ec_blocks: list[list[int]] = []
    pos = 0
    for num_blocks, data_count in groups:
        for _ in range(num_blocks):
            block = data_codewords[pos:pos + data_count]
            pos += data_count
            data_blocks.append(block)
            ec_blocks.append(reedsolomon.encode(block, ec_per_block))

    # Interleave data: column-major across blocks (block0[0], block1[0], ...).
    result: list[int] = []
    max_data = max(len(b) for b in data_blocks)
    for i in range(max_data):
        for block in data_blocks:
            if i < len(block):
                result.append(block[i])
    # Then interleave EC codewords the same way (every block has equal EC count).
    for i in range(ec_per_block):
        for block in ec_blocks:
            result.append(block[i])
    return result


def encode(text: str, ec_level: str = "M", encoding: str = "utf-8") -> EncodedData:
    """Run the full data-encoding pipeline and return interleaved codewords."""
    if ec_level not in tables.EC_LEVELS:
        raise ValueError(f"EC level must be one of {tables.EC_LEVELS}")

    data = text.encode(encoding)
    mode = analyze_mode(text)
    version = choose_version(data, text, mode, ec_level)

    buf = BitBuffer()
    buf.append_bits(mode, 4)
    count = len(text) if mode != tables.MODE_BYTE else len(data)
    buf.append_bits(count, tables.char_count_bits(version, mode))
    _encode_payload(buf, data, text, mode)

    total = tables.total_data_codewords(version, ec_level)
    _add_terminator_and_padding(buf, total)

    data_codewords = buf.to_codewords()
    codewords = _structure_codewords(data_codewords, version, ec_level)
    return EncodedData(version, ec_level, mode, codewords)
