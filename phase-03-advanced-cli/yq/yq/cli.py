"""Command-line interface for yq.

Usage::

    python -m yq [options] [FILTER] [FILE]

* ``FILTER`` is a jq-like expression (default ``.`` identity). As with jq it is
  the *first* positional argument.
* ``FILE`` is an optional path; if omitted (or ``-``) input is read from stdin.

Options:

* ``-o, --output-format {yaml,json}``  choose the output serialiser (default yaml)
* ``-c, --compact``                    compact JSON / flow-style YAML
* ``-i, --input-format {auto,yaml,json}`` advisory only; both are parsed by the
  YAML loader since JSON is valid YAML.

Exit codes (mirroring the codingchallenges.fyi spec):

* **0**  success
* **1**  invalid input document, or a query that fails at evaluation time
* **2**  usage / IO problems (bad flags, file not found, malformed query)
"""

from __future__ import annotations

import argparse
import sys
from typing import Any, List, Optional

from .convert import documents_to_yaml, to_json, to_yaml
from .errors import YqLoadError, YqQueryError
from .loader import load_documents
from .query import compile_query

EXIT_OK = 0
EXIT_INVALID = 1
EXIT_USAGE = 2


def _build_arg_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="yq",
        description="A from-scratch YAML processor with a jq-like query language.",
    )
    p.add_argument(
        "filter",
        nargs="?",
        default=".",
        help="jq-like filter expression (default: '.')",
    )
    p.add_argument(
        "file",
        nargs="?",
        help="input file; if omitted or '-', read from stdin",
    )
    p.add_argument(
        "-o",
        "--output-format",
        choices=("yaml", "json"),
        default="yaml",
        help="output serialiser (default: yaml)",
    )
    p.add_argument(
        "-i",
        "--input-format",
        choices=("auto", "yaml", "json"),
        default="auto",
        help="advisory input format; JSON is parsed as YAML either way",
    )
    p.add_argument(
        "-c",
        "--compact",
        action="store_true",
        help="compact output (dense JSON / flow-style YAML)",
    )
    return p


def _read_input(path: Optional[str], stdin) -> str:
    if path is None or path == "-":
        return stdin.read()
    with open(path, "r", encoding="utf-8") as fh:
        return fh.read()


def _emit(results: List[Any], fmt: str, compact: bool, stdout) -> None:
    if fmt == "json":
        for result in results:
            stdout.write(to_json(result, compact=compact))
            stdout.write("\n")
        return

    # YAML output.
    if not results:
        return
    if len(results) == 1:
        stdout.write(to_yaml(results[0], compact=compact))
    else:
        stdout.write(documents_to_yaml(results, compact=compact))


def main(argv: Optional[List[str]] = None, stdin=None, stdout=None) -> int:
    stdin = stdin if stdin is not None else sys.stdin
    stdout = stdout if stdout is not None else sys.stdout

    parser = _build_arg_parser()
    args = parser.parse_args(argv)

    # Compile the query first so a typo fails fast as a usage error.
    try:
        program = compile_query(args.filter)
    except YqQueryError as exc:
        print(f"yq: invalid query: {exc}", file=sys.stderr)
        return EXIT_USAGE

    try:
        text = _read_input(args.file, stdin)
    except OSError as exc:
        print(f"yq: could not read input: {exc}", file=sys.stderr)
        return EXIT_USAGE

    try:
        documents = load_documents(text)
    except YqLoadError as exc:
        print(f"yq: invalid input: {exc}", file=sys.stderr)
        return EXIT_INVALID

    try:
        results = [out for doc in documents for out in program.run(doc)]
    except YqQueryError as exc:
        print(f"yq: query error: {exc}", file=sys.stderr)
        return EXIT_INVALID

    _emit(results, args.output_format, args.compact, stdout)
    return EXIT_OK


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
