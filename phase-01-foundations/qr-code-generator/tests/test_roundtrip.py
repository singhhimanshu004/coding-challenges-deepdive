"""End-to-end round-trip test: generate a PNG and decode it back.

This is the strongest correctness proof we have — a third-party decoder reading
our symbol means every stage (encode → RS → matrix → mask → format info →
render) is correct. The decoder is optional: if neither OpenCV nor pyzbar is
installed, these tests skip with a clear message rather than failing.
"""

from __future__ import annotations

import os

import pytest

from qrgen import render
from qrgen.generator import make_qr


def _decode(path: str) -> str | None:
    """Decode a QR image. Prefer zbar (robust); fall back to OpenCV.

    zbar reads small/dense symbols reliably; OpenCV's detector is convenient
    (pure pip install) but flakier on tiny version-1/2 codes, so it is the
    fallback rather than the primary decoder.
    """
    try:
        from PIL import Image  # type: ignore
        from pyzbar.pyzbar import decode as zbar_decode  # type: ignore

        results = zbar_decode(Image.open(path))
        if results:
            return results[0].data.decode("utf-8")
    except ImportError:
        pass
    try:
        import cv2  # type: ignore

        img = cv2.imread(path)
        data, _, _ = cv2.QRCodeDetector().detectAndDecode(img)
        return data or None
    except ImportError:
        return None


def _decoder_available() -> bool:
    try:
        import pyzbar.pyzbar  # noqa: F401

        return True
    except ImportError:
        pass
    try:
        import cv2  # noqa: F401

        return True
    except ImportError:
        return False


requires_decoder = pytest.mark.skipif(
    not _decoder_available(), reason="no QR decoder (cv2/pyzbar) installed"
)


# Inputs chosen to land on version >= 2, where decoders are reliable. (Tiny
# version-1 symbols stress some detectors regardless of encoder correctness.)
ROUNDTRIP_CASES = [
    ("https://example.com/path?id=42", "L"),
    ("Hello, QR World! 1234567890", "M"),
    ("café ☕ — naïve façade", "M"),
    ("A" * 40, "H"),
    ("THE QUICK BROWN FOX 0123456789", "Q"),
]


@requires_decoder
@pytest.mark.parametrize("text,ec", ROUNDTRIP_CASES)
def test_roundtrip_decodes(text, ec, tmp_path):
    path = os.path.join(tmp_path, "qr.png")
    qr = make_qr(text, ec_level=ec)
    render.to_png(qr, path, scale=10)
    decoded = _decode(path)
    assert decoded == text, f"decoded {decoded!r} != {text!r} (v{qr.version})"


def test_ascii_and_unicode_render_nonempty():
    qr = make_qr("HELLO", ec_level="M")
    assert render.to_unicode(qr).strip()
    assert render.to_ascii(qr).strip()


def test_png_written(tmp_path):
    qr = make_qr("PNG TEST", ec_level="M")
    path = os.path.join(tmp_path, "out.png")
    render.to_png(qr, path, scale=4)
    assert os.path.getsize(path) > 0
