"""The lexer (a.k.a. tokenizer / scanner).

Stage one of the pipeline. It walks the raw input string **one character at a
time** and emits a flat list of :class:`Token` objects. It validates everything
that can be checked *locally* — valid number shapes, complete string escapes,
correctly spelled keywords — but it knows nothing about grammar (whether a
comma is in the right place is the parser's problem).

Design notes
------------
* We track ``line`` and ``column`` as we advance so every token (and every
  error) carries an exact source position.
* The number scanner mirrors the JSON spec's grammar diagram precisely, which
  is how we reject the classic gotchas: leading zeros (``01``), a bare ``-``,
  ``.5`` (no integer part), or ``1.`` (no fraction digits).
* Strings decode escapes — including ``\\uXXXX`` and UTF-16 surrogate pairs —
  into real Python ``str`` values here, so the parser deals only in finished
  strings.
"""

from __future__ import annotations

from typing import List

from .errors import JSONParseError
from .tokens import Token, TokenType

# Single-character tokens map cleanly to their type.
_STRUCTURAL = {
    "{": TokenType.LBRACE,
    "}": TokenType.RBRACE,
    "[": TokenType.LBRACKET,
    "]": TokenType.RBRACKET,
    ":": TokenType.COLON,
    ",": TokenType.COMMA,
}

# JSON permits exactly these four whitespace characters between tokens.
_WHITESPACE = {" ", "\t", "\n", "\r"}

# Two-character escape sequences inside strings.
_SIMPLE_ESCAPES = {
    '"': '"',
    "\\": "\\",
    "/": "/",
    "b": "\b",
    "f": "\f",
    "n": "\n",
    "r": "\r",
    "t": "\t",
}


class Lexer:
    """Turns a JSON document string into a list of tokens."""

    def __init__(self, text: str) -> None:
        self._text = text
        self._pos = 0          # index into the source string
        self._line = 1         # 1-based line number
        self._column = 1       # 1-based column number

    # -- low-level cursor helpers -------------------------------------------

    def _at_end(self) -> bool:
        return self._pos >= len(self._text)

    def _peek(self, offset: int = 0) -> str:
        """Return the character ``offset`` ahead without consuming it.

        Returns the empty string past end-of-input so callers can compare
        safely without an index check.
        """
        idx = self._pos + offset
        if idx >= len(self._text):
            return ""
        return self._text[idx]

    def _advance(self) -> str:
        """Consume and return the current character, updating line/column."""
        ch = self._text[self._pos]
        self._pos += 1
        if ch == "\n":
            self._line += 1
            self._column = 1
        else:
            self._column += 1
        return ch

    def _error(self, message: str) -> JSONParseError:
        return JSONParseError(message, self._line, self._column)

    # -- main entry point ----------------------------------------------------

    def tokenize(self) -> List[Token]:
        """Scan the whole document and return tokens terminated by EOF."""
        tokens: List[Token] = []
        while True:
            self._skip_whitespace()
            if self._at_end():
                tokens.append(Token(TokenType.EOF, None, self._line, self._column))
                return tokens
            tokens.append(self._next_token())

    def _skip_whitespace(self) -> None:
        while not self._at_end() and self._peek() in _WHITESPACE:
            self._advance()

    def _next_token(self) -> Token:
        line, column = self._line, self._column
        ch = self._peek()

        if ch in _STRUCTURAL:
            self._advance()
            return Token(_STRUCTURAL[ch], ch, line, column)
        if ch == '"':
            return self._scan_string()
        if ch == "-" or ch.isdigit():
            return self._scan_number()
        if ch.isalpha():
            return self._scan_keyword()

        raise self._error(f"Unexpected character {ch!r}")

    # -- strings -------------------------------------------------------------

    def _scan_string(self) -> Token:
        line, column = self._line, self._column
        self._advance()  # consume opening quote
        chars: List[str] = []

        while True:
            if self._at_end():
                raise JSONParseError("Unterminated string", line, column)
            ch = self._advance()

            if ch == '"':
                return Token(TokenType.STRING, "".join(chars), line, column)
            if ch == "\\":
                chars.append(self._scan_escape())
                continue
            # Raw control characters (U+0000–U+001F) must be escaped in JSON.
            if ord(ch) < 0x20:
                raise self._error(
                    f"Unescaped control character U+{ord(ch):04X} in string"
                )
            chars.append(ch)

    def _scan_escape(self) -> str:
        """Decode the character following a backslash."""
        if self._at_end():
            raise self._error("Unterminated escape sequence")
        esc = self._advance()
        if esc in _SIMPLE_ESCAPES:
            return _SIMPLE_ESCAPES[esc]
        if esc == "u":
            return self._scan_unicode_escape()
        raise self._error(f"Invalid escape sequence \\{esc}")

    def _scan_unicode_escape(self) -> str:
        r"""Decode a ``\uXXXX`` escape, joining UTF-16 surrogate pairs."""
        code = self._read_hex4()
        # High surrogate? Then a matching low surrogate must follow to form a
        # single astral-plane code point (e.g. emoji).
        if 0xD800 <= code <= 0xDBFF:
            if self._peek() == "\\" and self._peek(1) == "u":
                self._advance()  # backslash
                self._advance()  # u
                low = self._read_hex4()
                if not (0xDC00 <= low <= 0xDFFF):
                    raise self._error("Invalid low surrogate in \\u escape")
                combined = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)
                return chr(combined)
            raise self._error("Unpaired high surrogate in \\u escape")
        if 0xDC00 <= code <= 0xDFFF:
            raise self._error("Unpaired low surrogate in \\u escape")
        return chr(code)

    def _read_hex4(self) -> int:
        digits = []
        for _ in range(4):
            if self._at_end():
                raise self._error("Incomplete \\u escape (expected 4 hex digits)")
            ch = self._advance()
            if ch not in "0123456789abcdefABCDEF":
                raise self._error(f"Invalid hex digit {ch!r} in \\u escape")
            digits.append(ch)
        return int("".join(digits), 16)

    # -- numbers -------------------------------------------------------------

    def _scan_number(self) -> Token:
        r"""Scan a number following JSON's grammar:

            number = [ '-' ] int [ frac ] [ exp ]
            int    = '0' | digit1-9 *digit
            frac   = '.' 1*digit
            exp    = ('e' | 'E') [ '+' | '-' ] 1*digit
        """
        line, column = self._line, self._column
        start = self._pos

        if self._peek() == "-":
            self._advance()

        # Integer part: a lone '0', or a non-zero digit followed by more.
        if self._peek() == "0":
            self._advance()
            if self._peek().isdigit():
                raise self._error("Leading zeros are not allowed in numbers")
        elif self._peek().isdigit():
            while self._peek().isdigit():
                self._advance()
        else:
            raise self._error("Invalid number: expected a digit")

        is_float = False

        # Fraction part.
        if self._peek() == ".":
            is_float = True
            self._advance()
            if not self._peek().isdigit():
                raise self._error("Invalid number: expected digit after decimal point")
            while self._peek().isdigit():
                self._advance()

        # Exponent part.
        if self._peek() in ("e", "E"):
            is_float = True
            self._advance()
            if self._peek() in ("+", "-"):
                self._advance()
            if not self._peek().isdigit():
                raise self._error("Invalid number: expected digit in exponent")
            while self._peek().isdigit():
                self._advance()

        raw = self._text[start:self._pos]
        # Preserve integer-ness: integral JSON numbers become Python ints
        # (arbitrary precision), the rest become floats.
        value: object = float(raw) if is_float else int(raw)
        return Token(TokenType.NUMBER, value, line, column)

    # -- keywords ------------------------------------------------------------

    def _scan_keyword(self) -> Token:
        line, column = self._line, self._column
        for literal, ttype, value in (
            ("true", TokenType.TRUE, True),
            ("false", TokenType.FALSE, False),
            ("null", TokenType.NULL, None),
        ):
            if self._text.startswith(literal, self._pos):
                for _ in literal:
                    self._advance()
                return Token(ttype, value, line, column)
        # Grab the bad word so the message is useful.
        start = self._pos
        while not self._at_end() and self._peek().isalpha():
            self._advance()
        word = self._text[start:self._pos]
        raise JSONParseError(f"Unknown literal {word!r}", line, column)
