"""The jq-like query mini-language: lexer -> parser -> evaluator.

This is the **teaching core** of the challenge. Rather than shelling out to
``jq``, we build a tiny interpreter using the exact same three-stage skeleton as
the Phase 1 JSON parser:

    text  --(lexer)-->  tokens  --(parser)-->  AST  --(evaluator)-->  values

Supported expression grammar (a deliberate, learnable subset of jq)::

    program  := pipeline EOF
    pipeline := primary ( '|' primary )*
    primary  := builtin | path
    builtin  := 'length' | 'keys'
    path     := '.' step*
    step     := '.' IDENT
             |  '[' INT ']'          # index a sequence
             |  '[' STRING ']'       # index a mapping by key
             |  '[' ']'              # iterate a sequence/mapping
             |  '.' '[' ... ']'      # the same brackets after a dot

The single most important idea is the **value stream**. Every node, when
evaluated against one input value, produces a *stream* of zero or more output
values (a Python generator). ``.[]`` is what turns one value into many; ``|``
composes streams by feeding every output of the left side into the right side.
Identity ``.`` yields its input unchanged. This stream model is exactly how jq
works, and it is why ``.users[] | .name`` reads so naturally.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Iterator, List, Optional

from .errors import YqQueryError

__all__ = ["compile_query", "Program"]

BUILTINS = {"length", "keys"}


# --------------------------------------------------------------------------- #
# Stage 1: Lexer — text -> tokens
# --------------------------------------------------------------------------- #

@dataclass(frozen=True)
class Token:
    kind: str          # DOT PIPE LBRACKET RBRACKET INT STRING IDENT EOF
    value: Any = None
    pos: int = 0


class Lexer:
    """Turn a query string into a flat list of :class:`Token` objects."""

    def __init__(self, text: str) -> None:
        self._text = text
        self._i = 0
        self._n = len(text)

    def tokenize(self) -> List[Token]:
        tokens: List[Token] = []
        while self._i < self._n:
            ch = self._text[self._i]
            if ch.isspace():
                self._i += 1
                continue
            start = self._i
            if ch == ".":
                tokens.append(Token("DOT", ".", start))
                self._i += 1
            elif ch == "|":
                tokens.append(Token("PIPE", "|", start))
                self._i += 1
            elif ch == "[":
                tokens.append(Token("LBRACKET", "[", start))
                self._i += 1
            elif ch == "]":
                tokens.append(Token("RBRACKET", "]", start))
                self._i += 1
            elif ch == '"':
                tokens.append(self._read_string())
            elif ch == "-" or ch.isdigit():
                tokens.append(self._read_int())
            elif ch.isalpha() or ch == "_":
                tokens.append(self._read_ident())
            else:
                raise YqQueryError(f"unexpected character {ch!r} at position {start}")
        tokens.append(Token("EOF", None, self._n))
        return tokens

    def _read_string(self) -> Token:
        start = self._i
        self._i += 1  # opening quote
        chars: List[str] = []
        while self._i < self._n:
            ch = self._text[self._i]
            if ch == "\\":
                self._i += 1
                if self._i >= self._n:
                    break
                esc = self._text[self._i]
                chars.append({"n": "\n", "t": "\t", '"': '"', "\\": "\\"}.get(esc, esc))
                self._i += 1
                continue
            if ch == '"':
                self._i += 1
                return Token("STRING", "".join(chars), start)
            chars.append(ch)
            self._i += 1
        raise YqQueryError(f"unterminated string starting at position {start}")

    def _read_int(self) -> Token:
        start = self._i
        if self._text[self._i] == "-":
            self._i += 1
        digits_start = self._i
        while self._i < self._n and self._text[self._i].isdigit():
            self._i += 1
        if self._i == digits_start:
            raise YqQueryError(f"expected digits after '-' at position {start}")
        return Token("INT", int(self._text[start:self._i]), start)

    def _read_ident(self) -> Token:
        start = self._i
        while self._i < self._n and (
            self._text[self._i].isalnum() or self._text[self._i] == "_"
        ):
            self._i += 1
        return Token("IDENT", self._text[start:self._i], start)


# --------------------------------------------------------------------------- #
# Stage 2: Parser — tokens -> AST
#
# Each AST node knows how to evaluate itself against a single input value and
# yields a *stream* of output values.
# --------------------------------------------------------------------------- #

class Node:
    def eval(self, value: Any) -> Iterator[Any]:  # pragma: no cover - interface
        raise NotImplementedError


@dataclass
class Identity(Node):
    """``.`` — yield the input unchanged."""

    def eval(self, value: Any) -> Iterator[Any]:
        yield value


@dataclass
class Field(Node):
    """``.name`` / ``["name"]`` — look up a key in a mapping."""

    name: str

    def eval(self, value: Any) -> Iterator[Any]:
        if value is None:
            yield None
        elif isinstance(value, dict):
            yield value.get(self.name)
        else:
            raise YqQueryError(
                f"cannot index {_typename(value)} with {self.name!r}"
            )


@dataclass
class Index(Node):
    """``[i]`` — index a sequence (supports negatives, jq-style)."""

    index: int

    def eval(self, value: Any) -> Iterator[Any]:
        if value is None:
            yield None
        elif isinstance(value, list):
            i = self.index
            if -len(value) <= i < len(value):
                yield value[i]
            else:
                yield None
        else:
            raise YqQueryError(
                f"cannot index {_typename(value)} with number {self.index}"
            )


@dataclass
class Iterate(Node):
    """``.[]`` — explode a sequence or mapping into a stream of its members."""

    def eval(self, value: Any) -> Iterator[Any]:
        if isinstance(value, list):
            yield from value
        elif isinstance(value, dict):
            yield from value.values()
        else:
            raise YqQueryError(f"cannot iterate over {_typename(value)}")


@dataclass
class Pipe(Node):
    """``left | right`` — feed every output of *left* into *right*."""

    left: Node
    right: Node

    def eval(self, value: Any) -> Iterator[Any]:
        for intermediate in self.left.eval(value):
            yield from self.right.eval(intermediate)


@dataclass
class Path(Node):
    """A chain of steps applied left to right, threading the value stream."""

    steps: List[Node]

    def eval(self, value: Any) -> Iterator[Any]:
        stream: Iterator[Any] = iter((value,))
        for step in self.steps:
            stream = _flat_map(step, stream)
        yield from stream


@dataclass
class Builtin(Node):
    """``length`` / ``keys`` — a named function over the input value."""

    name: str

    def eval(self, value: Any) -> Iterator[Any]:
        if self.name == "length":
            yield _length(value)
        elif self.name == "keys":
            yield _keys(value)
        else:  # pragma: no cover - guarded by parser
            raise YqQueryError(f"unknown builtin {self.name!r}")


def _flat_map(step: Node, stream: Iterator[Any]) -> Iterator[Any]:
    for item in stream:
        yield from step.eval(item)


class Parser:
    """Recursive-descent parser producing an AST from the token list."""

    def __init__(self, tokens: List[Token]) -> None:
        self._tokens = tokens
        self._i = 0

    def parse(self) -> Node:
        node = self._parse_pipeline()
        self._expect("EOF")
        return node

    # ----- grammar rules -------------------------------------------------- #

    def _parse_pipeline(self) -> Node:
        node = self._parse_primary()
        while self._peek().kind == "PIPE":
            self._advance()
            right = self._parse_primary()
            node = Pipe(node, right)
        return node

    def _parse_primary(self) -> Node:
        tok = self._peek()
        if tok.kind == "IDENT":
            if tok.value not in BUILTINS:
                raise YqQueryError(
                    f"unknown function {tok.value!r}; expected one of "
                    f"{', '.join(sorted(BUILTINS))}"
                )
            self._advance()
            return Builtin(tok.value)
        if tok.kind == "DOT":
            return self._parse_path()
        raise YqQueryError(
            f"unexpected token {tok.value!r}; a filter must start with '.'"
        )

    def _parse_path(self) -> Node:
        self._expect("DOT")
        steps: List[Node] = []

        # What directly follows the leading dot.
        nxt = self._peek()
        if nxt.kind == "IDENT":
            self._advance()
            steps.append(Field(nxt.value))
        elif nxt.kind == "LBRACKET":
            steps.append(self._parse_bracket())
        elif nxt.kind == "STRING":
            self._advance()
            steps.append(Field(nxt.value))

        # Subsequent steps: more `.field`, `.[...]`, or bare `[...]`.
        while True:
            tok = self._peek()
            if tok.kind == "DOT":
                self._advance()
                follow = self._peek()
                if follow.kind == "IDENT":
                    self._advance()
                    steps.append(Field(follow.value))
                elif follow.kind == "LBRACKET":
                    steps.append(self._parse_bracket())
                elif follow.kind == "STRING":
                    self._advance()
                    steps.append(Field(follow.value))
                else:
                    raise YqQueryError(
                        "expected a field name or '[' after '.'"
                    )
            elif tok.kind == "LBRACKET":
                steps.append(self._parse_bracket())
            else:
                break

        if not steps:
            return Identity()
        if len(steps) == 1:
            return steps[0]
        return Path(steps)

    def _parse_bracket(self) -> Node:
        self._expect("LBRACKET")
        tok = self._peek()
        if tok.kind == "RBRACKET":
            self._advance()
            return Iterate()
        if tok.kind == "INT":
            self._advance()
            self._expect("RBRACKET")
            return Index(tok.value)
        if tok.kind == "STRING":
            self._advance()
            self._expect("RBRACKET")
            return Field(tok.value)
        raise YqQueryError(
            f"expected an index, string, or ']' inside '[ ]', got {tok.value!r}"
        )

    # ----- token helpers -------------------------------------------------- #

    def _peek(self) -> Token:
        return self._tokens[self._i]

    def _advance(self) -> Token:
        tok = self._tokens[self._i]
        self._i += 1
        return tok

    def _expect(self, kind: str) -> Token:
        tok = self._peek()
        if tok.kind != kind:
            raise YqQueryError(f"expected {kind} but found {tok.value!r}")
        return self._advance()


# --------------------------------------------------------------------------- #
# Stage 3: Evaluator entry point
# --------------------------------------------------------------------------- #

class Program:
    """A compiled query, ready to run against any number of documents."""

    def __init__(self, root: Node) -> None:
        self._root = root

    def run(self, value: Any) -> Iterator[Any]:
        """Evaluate the query against *value*, yielding the output stream."""
        yield from self._root.eval(value)


def compile_query(expression: str) -> Program:
    """Lex, parse and return a runnable :class:`Program` for *expression*."""
    tokens = Lexer(expression).tokenize()
    ast = Parser(tokens).parse()
    return Program(ast)


# --------------------------------------------------------------------------- #
# Builtin helpers (jq semantics)
# --------------------------------------------------------------------------- #

def _length(value: Any) -> Any:
    if value is None:
        return 0
    if isinstance(value, bool):
        raise YqQueryError("boolean has no length")
    if isinstance(value, (int, float)):
        return abs(value)
    if isinstance(value, (str, list, dict)):
        return len(value)
    raise YqQueryError(f"{_typename(value)} has no length")


def _keys(value: Any) -> Any:
    if isinstance(value, dict):
        return sorted(value.keys(), key=str)
    if isinstance(value, list):
        return list(range(len(value)))
    raise YqQueryError(f"{_typename(value)} has no keys")


def _typename(value: Any) -> str:
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "boolean"
    if isinstance(value, (int, float)):
        return "number"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        return "array"
    if isinstance(value, dict):
        return "object"
    return type(value).__name__
