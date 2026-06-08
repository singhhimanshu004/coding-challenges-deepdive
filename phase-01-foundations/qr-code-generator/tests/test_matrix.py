"""Structural tests for the matrix and masking stages."""

from __future__ import annotations

from qrgen import mask, tables
from qrgen.generator import make_qr
from qrgen.matrix import QRMatrix


def _is_finder(grid, top, left):
    """A correct 7x7 finder = dark border ring + 3x3 dark core."""
    expected = [
        [1, 1, 1, 1, 1, 1, 1],
        [1, 0, 0, 0, 0, 0, 1],
        [1, 0, 1, 1, 1, 0, 1],
        [1, 0, 1, 1, 1, 0, 1],
        [1, 0, 1, 1, 1, 0, 1],
        [1, 0, 0, 0, 0, 0, 1],
        [1, 1, 1, 1, 1, 1, 1],
    ]
    return all(
        grid[top + r][left + c] == expected[r][c]
        for r in range(7)
        for c in range(7)
    )


def test_three_finder_patterns_present():
    qr = make_qr("HELLO", ec_level="M")
    g, n = qr.grid, qr.size
    assert _is_finder(g, 0, 0)
    assert _is_finder(g, 0, n - 7)
    assert _is_finder(g, n - 7, 0)


def test_dark_module_is_dark():
    qr = make_qr("HELLO", ec_level="M")
    assert qr.grid[qr.size - 8][8] == 1


def test_timing_patterns_alternate():
    qr = make_qr("HELLO", ec_level="M")
    n = qr.size
    for i in range(8, n - 8):
        assert qr.grid[6][i] == (1 if i % 2 == 0 else 0)
        assert qr.grid[i][6] == (1 if i % 2 == 0 else 0)


def test_module_count_matches_version():
    for text, expected_min in [("HI", 21)]:
        qr = make_qr(text, ec_level="L")
        assert qr.size == tables.modules_for_version(qr.version)
        assert qr.size >= expected_min


def test_every_module_is_binary():
    qr = make_qr("café ☕", ec_level="Q")
    assert all(cell in (0, 1) for row in qr.grid for cell in row)


def test_mask_only_flips_data_modules():
    m = QRMatrix(2)
    m.build_function_patterns()
    m.place_data([0] * tables.total_data_codewords(2, "M"))
    masked = mask.apply_mask(m, 3)
    # Reserved (function) cells must be untouched by the mask.
    for r in range(m.size):
        for c in range(m.size):
            if m.reserved[r][c]:
                assert masked[r][c] == m.modules[r][c]


def test_penalty_scoring_prefers_balanced_grid():
    # A solid all-dark grid should score worse than a checkerboard.
    n = 21
    solid = [[1] * n for _ in range(n)]
    checker = [[(r + c) % 2 for c in range(n)] for r in range(n)]
    assert mask.penalty_score(solid) > mask.penalty_score(checker)


def test_choose_best_mask_returns_valid_index():
    qr = make_qr("HELLO WORLD", ec_level="M")
    assert 0 <= qr.mask <= 7
