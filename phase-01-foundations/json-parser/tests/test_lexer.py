"""Tests for the lexer (stage one: text -> tokens)."""

from __future__ import annotations

import pytest

from jsonparser.errors import JSONParseError
from jsonparser.lexer import Lexer
from jsonparser.tokens import TokenType


def types(text: str):
    return [t.type for t in Lexer(text).tokenize()]


def values(text: str):
    return [t.value for t in Lexer(text).tokenize() if t.type is not TokenType.EOF]


def test_structural_tokens():
    assert types("{}[]:,") == [
        TokenType.LBRACE,
        TokenType.RBRACE,
        TokenType.LBRACKET,
        TokenType.RBRACKET,
        TokenType.COLON,
        TokenType.COMMA,
        TokenType.EOF,
    ]


def test_whitespace_is_skipped():
    assert types('  \n\t { } \r\n') == [
        TokenType.LBRACE,
        TokenType.RBRACE,
        TokenType.EOF,
    ]


def test_keywords():
    assert values("true false null") == [True, False, None]


@pytest.mark.parametrize(
    "text,expected",
    [
        ("0", 0),
        ("-0", 0),
        ("42", 42),
        ("-7", -7),
        ("3.14", 3.14),
        ("-3.14", -3.14),
        ("6.022e23", 6.022e23),
        ("1E10", 1e10),
        ("2.5e-3", 2.5e-3),
        ("1e+2", 1e2),
    ],
)
def test_numbers(text, expected):
    assert values(text) == [expected]


def test_integers_stay_ints_floats_stay_floats():
    (val,) = values("42")
    assert isinstance(val, int)
    (val2,) = values("42.0")
    assert isinstance(val2, float)


@pytest.mark.parametrize("text", ["01", "00", "-01", "1.", ".5", "-", "1e", "1e+", "1.e3"])
def test_malformed_numbers_rejected(text):
    with pytest.raises(JSONParseError):
        Lexer(text).tokenize()


def test_simple_string_and_escapes():
    (val,) = values(r'"a\"b\\c\/d\n\t"')
    assert val == 'a"b\\c/d\n\t'


def test_unicode_escape():
    (val,) = values(r'"\u00e9"')
    assert val == "é"


def test_surrogate_pair_escape():
    # U+1F600 GRINNING FACE encoded as a UTF-16 surrogate pair.
    (val,) = values(r'"\ud83d\ude00"')
    assert val == "\U0001F600"


def test_unterminated_string():
    with pytest.raises(JSONParseError, match="Unterminated string"):
        Lexer('"oops').tokenize()


def test_invalid_escape():
    with pytest.raises(JSONParseError, match="Invalid escape"):
        Lexer(r'"\x"').tokenize()


def test_raw_control_char_rejected():
    with pytest.raises(JSONParseError, match="control character"):
        Lexer('"line\nbreak"').tokenize()


def test_unpaired_high_surrogate():
    with pytest.raises(JSONParseError, match="surrogate"):
        Lexer(r'"\ud83d"').tokenize()


def test_unknown_literal():
    with pytest.raises(JSONParseError, match="Unknown literal"):
        Lexer("tru").tokenize()


def test_token_positions():
    tokens = Lexer('{\n  "k": 1\n}').tokenize()
    # The string key sits on line 2.
    key = next(t for t in tokens if t.type is TokenType.STRING)
    assert key.line == 2
