"""Tests for YAML <-> JSON conversion helpers."""

from __future__ import annotations

import json

from yq.convert import documents_to_yaml, to_json, to_yaml
from yq.loader import load_documents


def test_yaml_to_json_roundtrips_through_python():
    doc = load_documents("a: 1\nb:\n  - x\n  - y\n")[0]
    out = to_json(doc)
    assert json.loads(out) == {"a": 1, "b": ["x", "y"]}


def test_json_compact_has_no_spaces():
    assert to_json({"a": 1, "b": 2}, compact=True) == '{"a":1,"b":2}'


def test_json_pretty_is_indented():
    out = to_json({"a": 1}, indent=2)
    assert out == '{\n  "a": 1\n}'


def test_json_input_to_yaml_output():
    doc = load_documents('{"name": "web", "ports": [80, 443]}')[0]
    text = to_yaml(doc)
    # Re-load to assert semantic equality regardless of formatting.
    assert load_documents(text)[0] == {"name": "web", "ports": [80, 443]}


def test_yaml_block_vs_flow_style():
    value = {"a": [1, 2]}
    block = to_yaml(value, compact=False)
    flow = to_yaml(value, compact=True)
    assert "\n- 1" in block          # block style breaks lines
    assert flow.strip().startswith("{")  # flow style is inline


def test_multi_document_output_has_separators():
    text = documents_to_yaml([{"n": 1}, {"n": 2}])
    assert text.count("---") == 2
    reloaded = load_documents(text)
    assert [d["n"] for d in reloaded] == [1, 2]


def test_unicode_preserved():
    assert "café" in to_yaml({"x": "café"})
    assert "café" in to_json({"x": "café"})
