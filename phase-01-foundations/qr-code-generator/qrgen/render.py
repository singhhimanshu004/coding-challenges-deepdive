"""Rendering — turn the 0/1 module grid into something you can see.

Two outputs:
    * ASCII / Unicode-block art for the terminal (no dependencies).
    * A PNG via Pillow (optional import — only needed for file output).

A "quiet zone" (light border, ≥4 modules per the standard) is added around the
symbol; scanners rely on it to find the symbol's edges.
"""

from __future__ import annotations

from .generator import QRCode

QUIET_ZONE = 4


def to_unicode(qr: QRCode, quiet: int = QUIET_ZONE) -> str:
    """Render using half-block characters so two rows share one text line.

    Each character cell shows an upper and lower module via ▀ ▄ █ and space,
    giving roughly square output in a typical terminal font.
    """
    size = qr.size
    n = size + 2 * quiet

    def dark(r: int, c: int) -> bool:
        rr, cc = r - quiet, c - quiet
        if 0 <= rr < size and 0 <= cc < size:
            return qr.grid[rr][cc] == 1
        return False  # quiet zone is light

    lines = []
    for r in range(0, n, 2):
        chars = []
        for c in range(n):
            top = dark(r, c)
            bottom = dark(r + 1, c) if r + 1 < n else False
            if top and bottom:
                chars.append("█")
            elif top:
                chars.append("▀")
            elif bottom:
                chars.append("▄")
            else:
                chars.append(" ")
        lines.append("".join(chars))
    return "\n".join(lines)


def to_ascii(qr: QRCode, quiet: int = QUIET_ZONE) -> str:
    """Render with two characters per module (more portable than half-blocks)."""
    size = qr.size
    lines = []
    for r in range(-quiet, size + quiet):
        row = []
        for c in range(-quiet, size + quiet):
            dark = 0 <= r < size and 0 <= c < size and qr.grid[r][c] == 1
            row.append("██" if dark else "  ")
        lines.append("".join(row))
    return "\n".join(lines)


def to_png(qr: QRCode, path: str, scale: int = 10, quiet: int = QUIET_ZONE) -> None:
    """Write a PNG. Pillow handles pixels only — encoding is all ours."""
    try:
        from PIL import Image
    except ImportError as exc:  # pragma: no cover - environment dependent
        raise RuntimeError(
            "Pillow is required for PNG output. Install with: pip install Pillow"
        ) from exc

    size = qr.size
    dim = (size + 2 * quiet) * scale
    img = Image.new("1", (dim, dim), 1)  # mode "1": 1-bit, 1 = white
    px = img.load()
    for r in range(size):
        for c in range(size):
            if qr.grid[r][c] == 1:
                x0 = (c + quiet) * scale
                y0 = (r + quiet) * scale
                for dy in range(scale):
                    for dx in range(scale):
                        px[x0 + dx, y0 + dy] = 0  # black
    img.save(path)
