"""Tests for the parser (stage two: tokens -> Python value).

We assert two things everywhere it matters:
1. Our parser produces the right Python structure.
2. Its accept/reject verdict agrees with the standard library's ``json`` —
   the ground truth for "is this valid JSON?".
"""

from __future__ import annotations

import json

import pytest

from jsonparser import JSONParseError, parse


# --------------------------------------------------------------------------
# Valid documents
# --------------------------------------------------------------------------

VALID_DOCS = [
    "{}",
    "[]",
    '{"key": "value"}',
    '{"key": "value", "key2": "value2"}',
    "[1, 2, 3]",
    '["a", true, false, null, 1.5]',
    '{"nested": {"a": [1, {"b": 2}]}}',
    '{"unicode": "\\u00e9\\u4e2d\\ud83d\\ude00"}',
    '{"num": -3.14e10, "int": 42, "zero": 0}',
    '  {  "spaced"  :  [  1  ,  2  ]  }  ',
    '{"empty_obj": {}, "empty_arr": []}',
    '"just a string"',
    "42",
    "true",
    "null",
]


@pytest.mark.parametrize("doc", VALID_DOCS)
def test_valid_matches_stdlib(doc):
    assert parse(doc) == json.loads(doc)


def test_returns_native_python_types():
    result = parse('{"s": "x", "i": 1, "f": 1.5, "b": true, "n": null, "a": []}')
    assert result == {"s": "x", "i": 1, "f": 1.5, "b": True, "n": None, "a": []}
    assert isinstance(result["i"], int)
    assert isinstance(result["f"], float)


# --------------------------------------------------------------------------
# Invalid documents
# --------------------------------------------------------------------------

INVALID_DOCS = [
    "",                       # empty input
    "   ",                    # only whitespace
    "{",                      # unclosed object
    "[",                      # unclosed array
    "}",                      # stray close
    '{"key": }',              # missing value
    '{"key" "value"}',        # missing colon
    '{key: "value"}',         # unquoted key
    "[1, 2,]",                # trailing comma (array)
    '{"a": 1,}',              # trailing comma (object)
    "[1 2]",                  # missing comma
    '{"a": 1 "b": 2}',        # missing comma (object)
    '{"a": 1} extra',         # trailing junk
    "[1, 2] [3]",             # two documents
    "'single quotes'",        # single quotes not allowed
    "{'a': 1}",               # single-quoted key
    "[,]",                    # leading comma
    "01",                     # leading zero
    '{"a": 1',                # unterminated object
]


@pytest.mark.parametrize("doc", INVALID_DOCS)
def test_invalid_rejected(doc):
    with pytest.raises(JSONParseError):
        parse(doc)


@pytest.mark.parametrize("doc", INVALID_DOCS)
def test_invalid_agrees_with_stdlib(doc):
    # Whatever we reject, stdlib json must also reject.
    with pytest.raises(json.JSONDecodeError):
        json.loads(doc)


# --------------------------------------------------------------------------
# Edge cases
# --------------------------------------------------------------------------

def test_deeply_nested_structure():
    # Recursive descent is bounded by the host language's call stack. ~200
    # levels sits comfortably inside CPython's default recursion limit; going
    # much deeper would need sys.setrecursionlimit (see the README trade-offs).
    depth = 200
    doc = "[" * depth + "]" * depth
    result = parse(doc)
    # Walk down to confirm the full nesting survived.
    node = result
    for _ in range(depth - 1):
        assert isinstance(node, list)
        node = node[0]
    assert node == []


def test_duplicate_keys_last_wins_by_default():
    # Matches stdlib json behaviour.
    assert parse('{"a": 1, "a": 2}') == {"a": 2}
    assert json.loads('{"a": 1, "a": 2}') == {"a": 2}


def test_duplicate_keys_can_be_rejected():
    with pytest.raises(JSONParseError, match="Duplicate object key"):
        parse('{"a": 1, "a": 2}', allow_duplicate_keys=False)


def test_large_integers_keep_precision():
    big = "123456789012345678901234567890"
    assert parse(big) == int(big)


def test_empty_string_value():
    assert parse('{"k": ""}') == {"k": ""}


def test_error_reports_line_and_column():
    with pytest.raises(JSONParseError) as info:
        parse('{\n  "a": 1\n  "b": 2\n}')
    err = info.value
    # The missing comma is detected at the "b" key on line 3.
    assert err.line == 3
