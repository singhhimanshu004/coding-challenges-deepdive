"""Data masking — making the symbol readable by balancing dark and light.

A naive layout can produce large solid blocks or patterns that look like the
finder marks, confusing a scanner. So QR XORs the *data* region (never the
function patterns) with one of eight fixed mask patterns, scores each result
with four penalty rules, and keeps the lowest-scoring (most readable) one.

The eight masks are simple functions of a module's (row, col). Each flips a
module when its predicate is true.
"""

from __future__ import annotations

from .matrix import QRMatrix

# MASK_FUNCS[i](row, col) is True where mask i flips the module.
MASK_FUNCS = [
    lambda r, c: (r + c) % 2 == 0,
    lambda r, c: r % 2 == 0,
    lambda r, c: c % 3 == 0,
    lambda r, c: (r + c) % 3 == 0,
    lambda r, c: (r // 2 + c // 3) % 2 == 0,
    lambda r, c: (r * c) % 2 + (r * c) % 3 == 0,
    lambda r, c: ((r * c) % 2 + (r * c) % 3) % 2 == 0,
    lambda r, c: ((r + c) % 2 + (r * c) % 3) % 2 == 0,
]


def apply_mask(matrix: QRMatrix, mask: int) -> list[list[int]]:
    """Return a fresh grid with `mask` XOR'd over all non-function modules."""
    func = MASK_FUNCS[mask]
    size = matrix.size
    out = [row[:] for row in matrix.modules]
    for r in range(size):
        for c in range(size):
            if not matrix.reserved[r][c] and func(r, c):
                out[r][c] ^= 1
    return out


def _penalty_rule1(grid: list[list[int]]) -> int:
    """Rule 1: runs of 5+ same-colored modules in a row/column (3 + extra)."""
    size = len(grid)
    score = 0
    for line in (grid, list(zip(*grid))):
        for row in line:
            run = 1
            for i in range(1, size):
                if row[i] == row[i - 1]:
                    run += 1
                else:
                    if run >= 5:
                        score += run - 2
                    run = 1
            if run >= 5:
                score += run - 2
    return score


def _penalty_rule2(grid: list[list[int]]) -> int:
    """Rule 2: every 2×2 block of one color costs 3 points."""
    size = len(grid)
    score = 0
    for r in range(size - 1):
        for c in range(size - 1):
            if grid[r][c] == grid[r][c + 1] == grid[r + 1][c] == grid[r + 1][c + 1]:
                score += 3
    return score


def _penalty_rule3(grid: list[list[int]]) -> int:
    """Rule 3: finder-like patterns (1:1:3:1:1 with a light run) cost 40 each."""
    size = len(grid)
    patterns = (
        [1, 0, 1, 1, 1, 0, 1, 0, 0, 0, 0],
        [0, 0, 0, 0, 1, 0, 1, 1, 1, 0, 1],
    )
    score = 0
    rows = grid
    cols = [list(col) for col in zip(*grid)]
    for lines in (rows, cols):
        for line in lines:
            for i in range(size - 10):
                window = list(line[i:i + 11])
                if window in patterns:
                    score += 40
    return score


def _penalty_rule4(grid: list[list[int]]) -> int:
    """Rule 4: penalize deviation of the dark-module ratio from 50%."""
    size = len(grid)
    dark = sum(sum(row) for row in grid)
    percent = dark * 100 / (size * size)
    # Nearest multiples of 5 below and above the dark percentage.
    lower = int(percent // 5) * 5
    upper = lower + 5
    return min(abs(lower - 50), abs(upper - 50)) // 5 * 10


def penalty_score(grid: list[list[int]]) -> int:
    """Total of all four penalty rules (lower = more scannable)."""
    return (
        _penalty_rule1(grid)
        + _penalty_rule2(grid)
        + _penalty_rule3(grid)
        + _penalty_rule4(grid)
    )


def choose_best_mask(matrix: QRMatrix) -> tuple[int, list[list[int]]]:
    """Try all 8 masks, score each, and return (best_mask, masked_grid)."""
    best_mask = 0
    best_grid: list[list[int]] | None = None
    best_score = None
    for mask in range(8):
        grid = apply_mask(matrix, mask)
        # Score with format info temporarily written so finder-adjacent runs are
        # judged fairly; we re-stamp the real format info after selection.
        score = penalty_score(grid)
        if best_score is None or score < best_score:
            best_score = score
            best_mask = mask
            best_grid = grid
    assert best_grid is not None
    return best_mask, best_grid
