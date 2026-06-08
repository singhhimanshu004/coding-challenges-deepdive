"""Galois Field GF(256) arithmetic — the bedrock of Reed–Solomon ECC.

QR codes correct errors using Reed–Solomon codes, and Reed–Solomon does all of
its arithmetic in a *finite field* called GF(256): the 256 possible byte values
{0, 1, ..., 255} form a closed algebraic system where you can add, subtract,
multiply and divide without ever leaving the set.

Why a field and not ordinary integer math?
    Error correction needs every non-zero element to have a multiplicative
    inverse (so division always works) and arithmetic that never overflows a
    byte. Ordinary integers fail both: 200 * 200 overflows a byte, and 6 has no
    integer inverse. A finite field gives us both properties for free.

How GF(256) is constructed:
    * Addition and subtraction are both XOR. There are no carries; 3 + 1 == 2.
    * Multiplication is polynomial multiplication modulo an irreducible
      "primitive polynomial". QR uses x^8 + x^4 + x^3 + x^2 + 1, i.e. 0x11D.
    * Every non-zero element is a power of a fixed generator g = 2. So we can
      precompute two lookup tables:
        - EXP[i]  = 2**i in the field  (the "antilog" table)
        - LOG[v]  = the i such that 2**i == v  (the "log" table)
      Then a*b = EXP[LOG[a] + LOG[b]] turns multiplication into addition of
      exponents, exactly like a slide rule. This is what makes RS fast.
"""

from __future__ import annotations

# QR's primitive polynomial: x^8 + x^4 + x^3 + x^2 + 1 == 0b1_0001_1101 == 0x11D.
PRIMITIVE = 0x11D

# Precomputed antilog (EXP) and log (LOG) tables, filled by _build_tables().
EXP = [0] * 512  # doubled so EXP[i + j] never indexes out of range (i, j < 255)
LOG = [0] * 256


def _build_tables() -> None:
    """Populate EXP/LOG by walking the powers of the generator g = 2."""
    x = 1
    for i in range(255):
        EXP[i] = x
        LOG[x] = i
        # Multiply by the generator (2): shift left, then reduce mod PRIMITIVE
        # if we overflow 8 bits. This is carry-less (XOR) reduction.
        x <<= 1
        if x & 0x100:
            x ^= PRIMITIVE
    # Mirror the first half so EXP[i + j] is valid for exponents up to 508.
    for i in range(255, 512):
        EXP[i] = EXP[i - 255]


_build_tables()


def add(a: int, b: int) -> int:
    """Field addition (and subtraction) — both are just XOR."""
    return a ^ b


def mul(a: int, b: int) -> int:
    """Field multiplication via the log/antilog slide-rule trick."""
    if a == 0 or b == 0:
        return 0
    return EXP[LOG[a] + LOG[b]]


def div(a: int, b: int) -> int:
    """Field division: subtract exponents (mod 255)."""
    if b == 0:
        raise ZeroDivisionError("division by zero in GF(256)")
    if a == 0:
        return 0
    return EXP[(LOG[a] - LOG[b]) % 255]


def pow_(a: int, power: int) -> int:
    """Raise a field element to an integer power."""
    if a == 0:
        return 0
    return EXP[(LOG[a] * power) % 255]


def inverse(a: int) -> int:
    """Multiplicative inverse: the element x with a * x == 1."""
    if a == 0:
        raise ZeroDivisionError("0 has no inverse in GF(256)")
    return EXP[(255 - LOG[a]) % 255]
