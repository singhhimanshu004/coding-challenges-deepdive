"""qrgen — a from-scratch QR code generator.

Public API:
    from qrgen import make_qr
    qr = make_qr("hello", ec_level="M")
    print(qr.size, qr.version, qr.mask)
"""

from __future__ import annotations

from .generator import QRCode, make_qr

__all__ = ["QRCode", "make_qr"]
__version__ = "1.0.0"
