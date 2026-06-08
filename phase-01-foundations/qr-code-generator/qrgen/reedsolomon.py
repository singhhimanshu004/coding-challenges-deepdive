"""Reed–Solomon error-correction encoding over GF(256).

Reed–Solomon treats a block of data codewords as the coefficients of a
polynomial M(x). To add `n` error-correction (EC) codewords we compute the
remainder of  M(x) * x^n  divided by a fixed *generator polynomial* g(x), all in
GF(256). Those `n` remainder bytes are the EC codewords; appended to the data
they let a decoder reconstruct the original even if some bytes are damaged.

Intuition (why polynomial division corrects errors):
    g(x) has `n` roots at consecutive powers of the field generator
    (2^0, 2^1, ... 2^(n-1)). Because the transmitted codeword T(x) is a multiple
    of g(x), it evaluates to 0 at every one of those roots. A decoder checks
    those evaluations ("syndromes"); any non-zero result reveals — and locates —
    errors. We only implement the *encoder* here (the QR generation side).

We represent polynomials as Python lists of GF(256) coefficients, highest power
first, e.g. [a, b, c] == a*x^2 + b*x + c.
"""

from __future__ import annotations

from . import galois


def poly_mul(p: list[int], q: list[int]) -> list[int]:
    """Multiply two polynomials in GF(256)."""
    result = [0] * (len(p) + len(q) - 1)
    for i, pi in enumerate(p):
        for j, qj in enumerate(q):
            result[i + j] ^= galois.mul(pi, qj)
    return result


def generator_poly(n: int) -> list[int]:
    """Build the degree-n RS generator polynomial.

    g(x) = (x - 2^0)(x - 2^1)...(x - 2^(n-1)). In GF(256) subtraction is XOR, so
    each factor is [1, 2^i]. We fold them together with polynomial multiply.
    """
    g = [1]
    for i in range(n):
        g = poly_mul(g, [1, galois.EXP[i]])
    return g


def encode(data: list[int], n_ec: int) -> list[int]:
    """Return the `n_ec` Reed–Solomon EC codewords for `data`.

    This is synthetic polynomial long division: divide  data * x^n_ec  by the
    generator polynomial and keep the remainder. We use the standard in-place
    LFSR-style algorithm so it runs in O(len(data) * n_ec).
    """
    gen = generator_poly(n_ec)
    # Start with the data followed by n_ec zero slots for the remainder.
    remainder = list(data) + [0] * n_ec
    for i in range(len(data)):
        coef = remainder[i]
        if coef == 0:
            continue  # multiplying the generator by 0 changes nothing
        # Subtract coef * gen aligned at position i (skip gen[0] which is 1).
        for j in range(1, len(gen)):
            remainder[i + j] ^= galois.mul(gen[j], coef)
    # The last n_ec entries are the remainder = the EC codewords.
    return remainder[len(data):]
