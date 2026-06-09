"""End-to-end tests for the CLI front end (exit codes, formats, stdin)."""

from __future__ import annotations

import io
import json

from yq.cli import main


def run_cli(argv, stdin_text=""):
    stdin = io.StringIO(stdin_text)
    stdout = io.StringIO()
    code = main(argv, stdin=stdin, stdout=stdout)
    return code, stdout.getvalue()


def test_identity_from_stdin_yaml_output():
    code, out = run_cli([".", "-"], "a: 1\n")
    assert code == 0
    assert "a: 1" in out


def test_field_query_json_output():
    code, out = run_cli([".name", "-o", "json"], "name: web\nport: 80\n")
    assert code == 0
    assert out.strip() == '"web"'


def test_iterate_produces_multiple_outputs():
    code, out = run_cli([".[]", "-o", "json", "-c"], "[1, 2, 3]")
    assert code == 0
    assert out.split() == ["1", "2", "3"]


def test_yaml_to_json_conversion():
    code, out = run_cli([".", "-o", "json", "-c"], "a: 1\nb: [2, 3]\n")
    assert code == 0
    assert json.loads(out) == {"a": 1, "b": [2, 3]}


def test_json_input_is_accepted():
    code, out = run_cli([".a", "-o", "json"], '{"a": 42}')
    assert code == 0
    assert out.strip() == "42"


def test_multi_document_input_each_processed():
    code, out = run_cli([".id", "-o", "json"], "id: 1\n---\nid: 2\n")
    assert code == 0
    assert out.split() == ["1", "2"]


def test_anchor_alias_query():
    text = (
        "defaults: &d\n"
        "  retries: 5\n"
        "prod:\n"
        "  <<: *d\n"
        "  region: us-east-1\n"
    )
    code, out = run_cli([".prod.retries", "-o", "json"], text)
    assert code == 0
    assert out.strip() == "5"


def test_invalid_yaml_exit_1():
    code, _ = run_cli([".", "-"], "a: [1, 2\n")
    assert code == 1


def test_invalid_query_exit_2():
    code, _ = run_cli(["..["], "a: 1\n")
    assert code == 2


def test_query_eval_error_exit_1():
    # Iterating over a scalar fails at evaluation time.
    code, _ = run_cli([".[]"], "5\n")
    assert code == 1


def test_default_filter_is_identity():
    code, out = run_cli([], "a: 1\n")
    assert code == 0
    assert "a: 1" in out
