"""Tests for the command-line interface."""

from __future__ import annotations

import io

import pytest

from jsonparser import cli


def test_valid_file_returns_zero(tmp_path, capsys):
    f = tmp_path / "ok.json"
    f.write_text('{"a": [1, 2, 3]}', encoding="utf-8")
    code = cli.main([str(f)])
    out = capsys.readouterr().out
    assert code == cli.EXIT_OK
    assert "'a'" in out


def test_invalid_file_returns_one(tmp_path, capsys):
    f = tmp_path / "bad.json"
    f.write_text('{"a": }', encoding="utf-8")
    code = cli.main([str(f)])
    err = capsys.readouterr().err
    assert code == cli.EXIT_INVALID
    assert "Invalid JSON" in err


def test_missing_file_returns_usage_error(capsys):
    code = cli.main(["/no/such/file.json"])
    err = capsys.readouterr().err
    assert code == cli.EXIT_USAGE
    assert "could not read input" in err


def test_quiet_suppresses_output(tmp_path, capsys):
    f = tmp_path / "ok.json"
    f.write_text("[1, 2]", encoding="utf-8")
    code = cli.main(["--quiet", str(f)])
    out = capsys.readouterr().out
    assert code == cli.EXIT_OK
    assert out == ""


def test_reads_from_stdin(monkeypatch, capsys):
    monkeypatch.setattr("sys.stdin", io.StringIO('{"from": "stdin"}'))
    code = cli.main([])
    assert code == cli.EXIT_OK
    assert "stdin" in capsys.readouterr().out


def test_no_duplicate_keys_flag(tmp_path, capsys):
    f = tmp_path / "dup.json"
    f.write_text('{"a": 1, "a": 2}', encoding="utf-8")
    assert cli.main([str(f)]) == cli.EXIT_OK
    assert cli.main(["--no-duplicate-keys", str(f)]) == cli.EXIT_INVALID
