"""Tests for the jq-like query lexer, parser and evaluator."""

from __future__ import annotations

import pytest

from yq.errors import YqQueryError
from yq.query import compile_query


def run(expr, value):
    return list(compile_query(expr).run(value))


# --- identity & field access ---------------------------------------------- #

def test_identity():
    assert run(".", {"a": 1}) == [{"a": 1}]


def test_field_access():
    assert run(".a", {"a": 1, "b": 2}) == [1]


def test_nested_field_access():
    assert run(".a.b", {"a": {"b": 7}}) == [7]


def test_missing_field_yields_null():
    assert run(".missing", {"a": 1}) == [None]


def test_field_on_null_is_null():
    assert run(".a.b", {"a": None}) == [None]


def test_bracket_string_key():
    assert run('.["a b"]', {"a b": 9}) == [9]


def test_field_on_scalar_errors():
    with pytest.raises(YqQueryError):
        run(".a", 5)


# --- indexing -------------------------------------------------------------- #

def test_index():
    assert run(".[1]", [10, 20, 30]) == [20]


def test_negative_index():
    assert run(".[-1]", [10, 20, 30]) == [30]


def test_index_out_of_range_is_null():
    assert run(".[9]", [1, 2]) == [None]


def test_index_into_field():
    assert run(".items[0]", {"items": ["x", "y"]}) == ["x"]


# --- iteration ------------------------------------------------------------- #

def test_iterate_list():
    assert run(".[]", [1, 2, 3]) == [1, 2, 3]


def test_iterate_object_values():
    assert run(".[]", {"a": 1, "b": 2}) == [1, 2]


def test_iterate_then_field():
    data = {"users": [{"name": "a"}, {"name": "b"}]}
    assert run(".users[] | .name", data) == ["a", "b"]


def test_iterate_over_scalar_errors():
    with pytest.raises(YqQueryError):
        run(".[]", 3)


# --- pipe ------------------------------------------------------------------ #

def test_pipe_threads_value():
    assert run(".a | .b", {"a": {"b": 42}}) == [42]


def test_pipe_with_iteration_and_length():
    data = {"xs": [[1], [1, 2], [1, 2, 3]]}
    assert run(".xs[] | length", data) == [1, 2, 3]


# --- builtins -------------------------------------------------------------- #

def test_length_of_list():
    assert run("length", [1, 2, 3]) == [3]


def test_length_of_string():
    assert run("length", "hello") == [5]


def test_length_of_null_is_zero():
    assert run("length", None) == [0]


def test_length_of_number_is_abs():
    assert run("length", -7) == [7]


def test_keys_of_object_sorted():
    assert run("keys", {"b": 1, "a": 2}) == [["a", "b"]]


def test_keys_of_array_is_indices():
    assert run("keys", [9, 8, 7]) == [[0, 1, 2]]


def test_unknown_builtin_errors():
    with pytest.raises(YqQueryError):
        compile_query("frobnicate")


# --- parse errors ---------------------------------------------------------- #

def test_trailing_pipe_errors():
    with pytest.raises(YqQueryError):
        compile_query(".a |")


def test_unterminated_string_errors():
    with pytest.raises(YqQueryError):
        compile_query('.["a')


def test_bad_leading_token_errors():
    with pytest.raises(YqQueryError):
        compile_query("| .a")
