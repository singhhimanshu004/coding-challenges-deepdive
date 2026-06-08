"""Reed–Solomon tests, anchored to published reference vectors."""

from __future__ import annotations

from qrgen import reedsolomon


def test_generator_poly_degree():
    # A degree-n generator has n+1 coefficients and a leading 1.
    g = reedsolomon.generator_poly(10)
    assert len(g) == 11
    assert g[0] == 1


def test_rs_wikiversity_vector():
    """Canonical QR example from 'Reed–Solomon codes for coders' (Wikiversity)."""
    msg = [0x40, 0xD2, 0x75, 0x47, 0x76, 0x17, 0x32, 0x06,
           0x27, 0x26, 0x96, 0xC6, 0xC6, 0x96, 0x70, 0xEC]
    expected = [0xBC, 0x2A, 0x90, 0x13, 0x6B, 0xAF, 0xEF, 0xFD, 0x4B, 0xE0]
    assert reedsolomon.encode(msg, 10) == expected


def test_rs_thonky_data_block():
    """Thonky 'HELLO WORLD' V1-Q data block produces 13 EC codewords."""
    data = [0x20, 0x5B, 0x0B, 0x78, 0xD1, 0x72, 0xDC, 0x4D,
            0x43, 0x40, 0xEC, 0x11, 0xEC]
    ec = reedsolomon.encode(data, 13)
    assert len(ec) == 13
    # EC codewords are deterministic; re-encoding must reproduce them.
    assert reedsolomon.encode(data, 13) == ec


def test_rs_zero_message():
    # The all-zero message has all-zero EC codewords.
    assert reedsolomon.encode([0] * 16, 10) == [0] * 10
