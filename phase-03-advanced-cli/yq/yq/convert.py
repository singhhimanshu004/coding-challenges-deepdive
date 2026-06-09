"""Serialisation: Python objects -> YAML or JSON text.

Because the loader normalises every input (YAML *or* JSON) into plain Python
objects, conversion is just a matter of choosing an output serialiser:

* :func:`to_json` uses the standard library ``json`` module.
* :func:`to_yaml` uses PyYAML's ``safe_dump``.

This is the practical pay-off of "YAML is a JSON superset": once data is in the
common Python representation, YAML->JSON and JSON->YAML are the same operation
with a different writer at the end.
"""

from __future__ import annotations

import json
from typing import Any

import yaml

__all__ = ["to_json", "to_yaml"]


def to_json(value: Any, *, compact: bool = False, indent: int = 2) -> str:
    """Serialise *value* as JSON.

    With ``compact=True`` you get a single dense line (``,``/``:`` separators);
    otherwise the output is pretty-printed with *indent* spaces. ``ensure_ascii``
    is disabled so non-ASCII text stays readable.
    """
    if compact:
        return json.dumps(value, separators=(",", ":"), ensure_ascii=False)
    return json.dumps(value, indent=indent, ensure_ascii=False, sort_keys=False)


def to_yaml(value: Any, *, compact: bool = False) -> str:
    """Serialise *value* as YAML.

    ``compact=True`` selects flow style (``{a: 1, b: [1, 2]}``); the default is
    the more readable block style. Keys are left in insertion order and Unicode
    is preserved.
    """
    text = yaml.safe_dump(
        value,
        default_flow_style=compact,
        sort_keys=False,
        allow_unicode=True,
    )
    # PyYAML appends an explicit "...\n" end-marker for bare scalars; drop it so
    # scalar results print as a clean single line (matching jq/yq ergonomics).
    if text.endswith("...\n"):
        text = text[: -len("...\n")]
    return text


def documents_to_yaml(values, *, compact: bool = False) -> str:
    """Serialise a list of documents as a multi-document YAML stream."""
    return yaml.safe_dump_all(
        values,
        default_flow_style=compact,
        sort_keys=False,
        allow_unicode=True,
        explicit_start=True,
    )
