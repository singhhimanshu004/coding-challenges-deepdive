"""Top-level orchestration — text in, finished QR module grid out."""

from __future__ import annotations

from dataclasses import dataclass

from . import encode, mask
from .matrix import QRMatrix


@dataclass
class QRCode:
    """A finished QR symbol ready to render."""

    version: int
    ec_level: str
    mask: int
    size: int
    grid: list[list[int]]  # size×size of 0 (light) / 1 (dark)

    def __getitem__(self, rc: tuple[int, int]) -> int:
        r, c = rc
        return self.grid[r][c]


def make_qr(text: str, ec_level: str = "M", encoding: str = "utf-8") -> QRCode:
    """Build a complete QR code for `text` at the given EC level.

    Pipeline: encode → place data → choose mask → stamp format/version info.
    """
    encoded = encode.encode(text, ec_level=ec_level, encoding=encoding)

    matrix = QRMatrix(encoded.version)
    matrix.build_function_patterns()
    matrix.place_data(encoded.codewords)

    best_mask, masked_grid = mask.choose_best_mask(matrix)

    # Commit the winning mask, then write format/version info (which is *not*
    # masked) on top of it.
    matrix.modules = masked_grid
    matrix.write_format_info(ec_level, best_mask)
    matrix.write_version_info()

    # All cells are now 0/1 — flatten Nones (there should be none) to light.
    grid = [[m if m is not None else 0 for m in row] for row in matrix.modules]
    return QRCode(encoded.version, ec_level, best_mask, matrix.size, grid)
