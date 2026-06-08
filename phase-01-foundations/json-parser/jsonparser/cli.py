"""Command-line interface for the JSON parser.

Usage::

    python -m jsonparser [FILE]        # parse FILE
    cat data.json | python -m jsonparser   # parse stdin

Contract (mirrors the codingchallenges.fyi spec):

* Exit code **0** when the input is valid JSON.
* Exit code **1** when the input is invalid (with a helpful message on stderr).
* Exit code **2** for usage / IO problems (e.g. file not found).

On success it pretty-prints the parsed Python structure so you can eyeball the
result; pass ``--quiet`` to suppress that and use it purely as a validator.
"""

from __future__ import annotations

import argparse
import sys
from pprint import pformat
from typing import List, Optional

from .errors import JSONParseError
from .parser import parse

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2


def _build_arg_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="jsonparser",
        description="Validate and parse JSON (a from-scratch implementation).",
    )
    p.add_argument(
        "file",
        nargs="?",
        help="path to a JSON file; if omitted, read from stdin",
    )
    p.add_argument(
        "-q",
        "--quiet",
        action="store_true",
        help="do not print the parsed structure; only set the exit code",
    )
    p.add_argument(
        "--no-duplicate-keys",
        action="store_true",
        help="reject objects that contain duplicate keys",
    )
    return p


def _read_input(path: Optional[str]) -> str:
    if path is None:
        return sys.stdin.read()
    with open(path, "r", encoding="utf-8") as fh:
        return fh.read()


def main(argv: Optional[List[str]] = None) -> int:
    args = _build_arg_parser().parse_args(argv)

    try:
        text = _read_input(args.file)
    except OSError as exc:
        print(f"error: could not read input: {exc}", file=sys.stderr)
        return EXIT_USAGE

    try:
        value = parse(text, allow_duplicate_keys=not args.no_duplicate_keys)
    except JSONParseError as exc:
        print(f"Invalid JSON: {exc}", file=sys.stderr)
        return EXIT_INVALID

    if not args.quiet:
        print(pformat(value))
    return EXIT_OK


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
