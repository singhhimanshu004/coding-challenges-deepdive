"""Token definitions for the JSON lexer.

A *token* is the smallest meaningful unit of the input — the lexer's job is to
group raw characters into these so the parser never has to think about
individual bytes again. This is the classic two-stage pipeline:

    raw text  ──(lexer)──►  tokens  ──(parser)──►  Python value

Each token remembers where it started (line/column) so that any error the
parser raises can point the user at the exact spot in their file.
"""

from __future__ import annotations

import enum
from dataclasses import dataclass
from typing import Any


class TokenType(enum.Enum):
    """The complete set of lexical categories in JSON.

    JSON's grammar is tiny — six structural characters, three literal
    keywords, plus strings and numbers — which is exactly what makes it the
    perfect first parser.
    """

    LBRACE = "{"          # begins an object
    RBRACE = "}"          # ends an object
    LBRACKET = "["        # begins an array
    RBRACKET = "]"        # ends an array
    COLON = ":"           # separates a key from its value
    COMMA = ","           # separates members / elements
    STRING = "STRING"     # "..."
    NUMBER = "NUMBER"     # 42, -3.14, 6.022e23
    TRUE = "true"
    FALSE = "false"
    NULL = "null"
    EOF = "EOF"           # sentinel: no more input


@dataclass(frozen=True)
class Token:
    """A single lexical token.

    `value` holds the *decoded* payload for strings (escapes resolved) and the
    numeric value for numbers; for structural tokens and keywords it is the
    Python value the token represents (or ``None``).
    """

    type: TokenType
    value: Any
    line: int
    column: int

    def __repr__(self) -> str:  # pragma: no cover - debug aid only
        return f"Token({self.type.name}, {self.value!r}, {self.line}:{self.column})"
