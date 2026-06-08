"""Command-line interface: ``python -m qrgen``.

Examples:
    python -m qrgen "https://example.com" -o out.png
    python -m qrgen "HELLO" --ec Q --ascii
    echo "piped text" | python -m qrgen -o out.png
"""

from __future__ import annotations

import argparse
import sys

from . import render
from .generator import make_qr


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="qrgen", description="Generate a QR code from scratch."
    )
    p.add_argument("text", nargs="?", help="text to encode (reads stdin if omitted)")
    p.add_argument("-o", "--output", help="write a PNG to this path")
    p.add_argument(
        "--ec",
        choices=("L", "M", "Q", "H"),
        default="M",
        help="error-correction level (default: M)",
    )
    p.add_argument("--scale", type=int, default=10, help="PNG pixels per module")
    p.add_argument(
        "--ascii",
        action="store_true",
        help="print full-block ASCII instead of half-block Unicode",
    )
    p.add_argument(
        "--quiet-terminal",
        action="store_true",
        help="do not print the QR to the terminal",
    )
    p.add_argument(
        "--encoding", default="utf-8", help="text encoding for byte mode"
    )
    return p


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)

    text = args.text
    if text is None:
        text = sys.stdin.read().rstrip("\n")
    if text == "" and args.text is None:
        build_parser().error("no input text provided")

    try:
        qr = make_qr(text, ec_level=args.ec, encoding=args.encoding)
    except (ValueError, LookupError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if not args.quiet_terminal:
        out = render.to_ascii(qr) if args.ascii else render.to_unicode(qr)
        print(out)
        print(
            f"version {qr.version}  ·  EC {qr.ec_level}  ·  "
            f"mask {qr.mask}  ·  {qr.size}×{qr.size} modules",
            file=sys.stderr,
        )

    if args.output:
        render.to_png(qr, args.output, scale=args.scale)
        print(f"wrote {args.output}", file=sys.stderr)

    return 0
