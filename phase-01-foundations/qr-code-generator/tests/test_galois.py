"""Unit tests for GF(256) arithmetic."""

from __future__ import annotations

import pytest

from qrgen import galois


def test_tables_are_complete_and_consistent():
    # Every non-zero element appears exactly once in the antilog cycle.
    assert sorted(galois.EXP[:255]) == list(range(1, 256))
    # LOG and EXP are inverses for all non-zero elements.
    for v in range(1, 256):
        assert galois.EXP[galois.LOG[v]] == v


def test_add_is_xor():
    assert galois.add(0, 0) == 0
    assert galois.add(0xAB, 0xCD) == 0xAB ^ 0xCD
    # Addition is its own inverse: a + a == 0.
    for a in (1, 17, 200, 255):
        assert galois.add(a, a) == 0


def test_mul_identity_and_zero():
    for a in (0, 1, 42, 255):
        assert galois.mul(a, 1) == a
        assert galois.mul(a, 0) == 0


def test_mul_is_commutative_and_associative():
    a, b, c = 0x53, 0xCA, 0x1F
    assert galois.mul(a, b) == galois.mul(b, a)
    assert galois.mul(galois.mul(a, b), c) == galois.mul(a, galois.mul(b, c))


def test_div_inverts_mul():
    for a in (1, 5, 100, 255):
        for b in (1, 7, 130, 254):
            assert galois.div(galois.mul(a, b), b) == a


def test_inverse():
    for a in range(1, 256):
        assert galois.mul(a, galois.inverse(a)) == 1


def test_div_by_zero_raises():
    with pytest.raises(ZeroDivisionError):
        galois.div(5, 0)
    with pytest.raises(ZeroDivisionError):
        galois.inverse(0)


def test_pow():
    assert galois.pow_(2, 0) == 1
    assert galois.pow_(2, 1) == 2
    # 2^8 in GF(256) reduces via the primitive polynomial 0x11D.
    assert galois.pow_(2, 8) == (0x11D ^ 0x100)
