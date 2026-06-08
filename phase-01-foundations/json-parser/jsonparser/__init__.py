"""A from-scratch JSON parser — the Phase 1 capstone challenge.

Public API mirrors the standard library's ``json`` module at a small scale::

    from jsonparser import parse, JSONParseError
    value = parse('{"hello": "world"}')

The package is split into three teaching-sized pieces:

* :mod:`jsonparser.lexer`  — text   -> tokens
* :mod:`jsonparser.parser` — tokens -> Python value
* :mod:`jsonparser.cli`    — command-line front end
"""

from __future__ import annotations

from .errors import JSONParseError
from .lexer import Lexer
from .parser import Parser, parse
from .tokens import Token, TokenType

__all__ = [
    "parse",
    "Parser",
    "Lexer",
    "Token",
    "TokenType",
    "JSONParseError",
]
