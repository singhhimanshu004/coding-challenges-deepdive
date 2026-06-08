"""The recursive-descent parser.

Stage two of the pipeline. It consumes the flat token list produced by the
lexer and builds a real Python value (``dict``, ``list``, ``str``, ``int``,
``float``, ``bool`` or ``None``).

Why *recursive descent*?
------------------------
JSON's grammar is recursive: a value can be an array, whose elements are
values, which can themselves be arrays... A recursive-descent parser mirrors
that structure **directly** — one function per grammar rule, and the call
stack does the bookkeeping of "where am I nested". It is the most readable way
to turn a grammar into code, which is exactly why it's the foundation reused
later for jq/yq, the calculator, and the Lisp interpreter.

The grammar we implement (JSON, RFC 8259)::

    json    = value
    value   = object | array | STRING | NUMBER | true | false | null
    object  = '{' [ member (',' member)* ] '}'
    member  = STRING ':' value
    array   = '[' [ value (',' value)* ] ']'

Each rule below maps to a ``_parse_*`` method. The single-token lookahead
(`_peek`) plus "consume exactly what the grammar expects next" (`_expect`) is
all the machinery we need — no backtracking required, because JSON is an
LL(1) grammar (the next token always tells us unambiguously what rule applies).
"""

from __future__ import annotations

from typing import Any, Dict, List

from .errors import JSONParseError
from .lexer import Lexer
from .tokens import Token, TokenType

# Tokens that can legally start a value. Used for friendlier error messages.
_VALUE_STARTERS = {
    TokenType.LBRACE,
    TokenType.LBRACKET,
    TokenType.STRING,
    TokenType.NUMBER,
    TokenType.TRUE,
    TokenType.FALSE,
    TokenType.NULL,
}


class Parser:
    """Recursive-descent parser over a token list."""

    def __init__(self, tokens: List[Token], *, allow_duplicate_keys: bool = True) -> None:
        self._tokens = tokens
        self._pos = 0
        self._allow_duplicate_keys = allow_duplicate_keys

    # -- token cursor helpers ------------------------------------------------

    def _peek(self) -> Token:
        return self._tokens[self._pos]

    def _advance(self) -> Token:
        token = self._tokens[self._pos]
        # Never advance past EOF; it's the permanent sentinel.
        if token.type is not TokenType.EOF:
            self._pos += 1
        return token

    def _expect(self, ttype: TokenType, what: str) -> Token:
        token = self._peek()
        if token.type is not ttype:
            raise JSONParseError(
                f"Expected {what} but found {self._describe(token)}",
                token.line,
                token.column,
            )
        return self._advance()

    @staticmethod
    def _describe(token: Token) -> str:
        if token.type is TokenType.EOF:
            return "end of input"
        if token.type is TokenType.STRING:
            return f"string {token.value!r}"
        if token.type is TokenType.NUMBER:
            return f"number {token.value}"
        return repr(token.value)

    # -- entry point ---------------------------------------------------------

    def parse(self) -> Any:
        """Parse a complete document and ensure no trailing junk remains."""
        # An empty document (only EOF) is invalid JSON.
        if self._peek().type is TokenType.EOF:
            token = self._peek()
            raise JSONParseError("Unexpected end of input (empty document)",
                                 token.line, token.column)
        value = self._parse_value()
        trailing = self._peek()
        if trailing.type is not TokenType.EOF:
            raise JSONParseError(
                f"Unexpected trailing content {self._describe(trailing)}",
                trailing.line,
                trailing.column,
            )
        return value

    # -- grammar rules -------------------------------------------------------

    def _parse_value(self) -> Any:
        token = self._peek()
        ttype = token.type
        if ttype is TokenType.LBRACE:
            return self._parse_object()
        if ttype is TokenType.LBRACKET:
            return self._parse_array()
        if ttype in (TokenType.STRING, TokenType.NUMBER):
            return self._advance().value
        if ttype in (TokenType.TRUE, TokenType.FALSE, TokenType.NULL):
            return self._advance().value
        raise JSONParseError(
            f"Expected a value but found {self._describe(token)}",
            token.line,
            token.column,
        )

    def _parse_object(self) -> Dict[str, Any]:
        self._expect(TokenType.LBRACE, "'{'")
        obj: Dict[str, Any] = {}

        # Empty object: '{}'.
        if self._peek().type is TokenType.RBRACE:
            self._advance()
            return obj

        while True:
            key_token = self._expect(TokenType.STRING, "string key")
            key = key_token.value
            if not self._allow_duplicate_keys and key in obj:
                raise JSONParseError(
                    f"Duplicate object key {key!r}",
                    key_token.line,
                    key_token.column,
                )
            self._expect(TokenType.COLON, "':'")
            obj[key] = self._parse_value()

            sep = self._peek()
            if sep.type is TokenType.COMMA:
                self._advance()
                # A comma must be followed by another member — reject the
                # trailing-comma case '{"a":1,}'.
                if self._peek().type is TokenType.RBRACE:
                    bad = self._peek()
                    raise JSONParseError("Trailing comma in object",
                                         bad.line, bad.column)
                continue
            if sep.type is TokenType.RBRACE:
                self._advance()
                return obj
            raise JSONParseError(
                f"Expected ',' or '}}' in object but found {self._describe(sep)}",
                sep.line,
                sep.column,
            )

    def _parse_array(self) -> List[Any]:
        self._expect(TokenType.LBRACKET, "'['")
        arr: List[Any] = []

        # Empty array: '[]'.
        if self._peek().type is TokenType.RBRACKET:
            self._advance()
            return arr

        while True:
            arr.append(self._parse_value())

            sep = self._peek()
            if sep.type is TokenType.COMMA:
                self._advance()
                # Reject the trailing-comma case '[1,]'.
                if self._peek().type is TokenType.RBRACKET:
                    bad = self._peek()
                    raise JSONParseError("Trailing comma in array",
                                         bad.line, bad.column)
                continue
            if sep.type is TokenType.RBRACKET:
                self._advance()
                return arr
            raise JSONParseError(
                f"Expected ',' or ']' in array but found {self._describe(sep)}",
                sep.line,
                sep.column,
            )


def parse(text: str, *, allow_duplicate_keys: bool = True) -> Any:
    """Parse a JSON document string into a Python value.

    This is the one-call front door that wires the lexer and parser together —
    the same shape as the standard library's ``json.loads``.

    Args:
        text: The raw JSON document.
        allow_duplicate_keys: When ``False``, a repeated object key raises
            instead of silently keeping the last value (RFC 8259 permits
            either behaviour; last-wins is the default, matching ``json``).

    Raises:
        JSONParseError: If ``text`` is not valid JSON.
    """
    tokens = Lexer(text).tokenize()
    return Parser(tokens, allow_duplicate_keys=allow_duplicate_keys).parse()
