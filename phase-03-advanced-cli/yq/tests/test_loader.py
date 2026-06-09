"""Tests for the YAML/JSON loader and the data-model mapping."""

from __future__ import annotations

import pytest

from yq.errors import YqLoadError
from yq.loader import load_documents


def test_scalars_map_to_python_types():
    doc = load_documents(
        "a_string: hello\n"
        "an_int: 42\n"
        "a_float: 3.14\n"
        "a_bool: true\n"
        "a_null: null\n"
    )[0]
    assert doc == {
        "a_string": "hello",
        "an_int": 42,
        "a_float": 3.14,
        "a_bool": True,
        "a_null": None,
    }


def test_sequence_becomes_list():
    assert load_documents("- 1\n- 2\n- 3\n")[0] == [1, 2, 3]


def test_mapping_becomes_dict():
    assert load_documents("k: v\n")[0] == {"k": "v"}


def test_json_is_valid_yaml():
    # JSON is a subset of YAML, so the same loader reads it.
    assert load_documents('{"a": [1, 2], "b": "x"}')[0] == {"a": [1, 2], "b": "x"}


def test_flow_style_equals_block_style():
    block = load_documents("a:\n  - 1\n  - 2\n")[0]
    flow = load_documents("a: [1, 2]\n")[0]
    assert block == flow


def test_anchor_and_alias_resolved():
    text = (
        "base: &base\n"
        "  retries: 5\n"
        "prod:\n"
        "  <<: *base\n"
        "  region: us-east-1\n"
    )
    doc = load_documents(text)[0]
    assert doc["prod"] == {"retries": 5, "region": "us-east-1"}
    assert doc["base"] == {"retries": 5}


def test_multi_document_stream():
    docs = load_documents("name: a\n---\nname: b\n---\nname: c\n")
    assert [d["name"] for d in docs] == ["a", "b", "c"]


def test_empty_input_is_single_null_document():
    assert load_documents("") == [None]


def test_invalid_yaml_raises():
    with pytest.raises(YqLoadError):
        load_documents("a: [1, 2\n")
