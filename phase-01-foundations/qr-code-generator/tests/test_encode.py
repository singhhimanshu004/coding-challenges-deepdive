"""Encoding-pipeline tests: mode analysis, version selection, codeword stream."""

from __future__ import annotations

import pytest

from qrgen import encode, tables


def test_mode_analysis():
    assert encode.analyze_mode("12345") == tables.MODE_NUMERIC
    assert encode.analyze_mode("HELLO WORLD") == tables.MODE_ALPHANUMERIC
    assert encode.analyze_mode("hello") == tables.MODE_BYTE  # lowercase -> byte
    assert encode.analyze_mode("café") == tables.MODE_BYTE


def test_thonky_reference_codewords():
    """The full interleaved stream must start with the known data codewords."""
    enc = encode.encode("HELLO WORLD", ec_level="Q")
    assert enc.version == 1
    assert enc.codewords[:13] == [
        0x20, 0x5B, 0x0B, 0x78, 0xD1, 0x72, 0xDC, 0x4D,
        0x43, 0x40, 0xEC, 0x11, 0xEC,
    ]
    # 13 data + 13 EC = 26 total codewords for V1-Q.
    assert len(enc.codewords) == 26


def test_version_grows_with_input():
    small = encode.encode("HI", ec_level="L")
    big = encode.encode("A" * 60, ec_level="L")
    assert small.version < big.version


def test_higher_ec_needs_bigger_version():
    # Same payload, stronger EC -> may require a larger version (never smaller).
    payload = "A" * 30
    vl = encode.encode(payload, ec_level="L").version
    vh = encode.encode(payload, ec_level="H").version
    assert vh >= vl


def test_empty_string_encodes():
    enc = encode.encode("", ec_level="M")
    assert enc.version == 1
    # Codeword count equals total codewords for V1-M (16 data + 10 EC).
    assert len(enc.codewords) == 26


def test_too_large_raises():
    with pytest.raises(ValueError):
        encode.encode("x" * 5000, ec_level="H")


def test_padding_uses_alternating_pad_bytes():
    enc = encode.encode("A", ec_level="L")
    # After the short payload the stream pads with 0xEC / 0x11 alternation.
    assert 0xEC in enc.codewords and 0x11 in enc.codewords
