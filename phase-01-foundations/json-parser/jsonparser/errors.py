"""Error types for the JSON parser.

A single, well-defined exception type (`JSONParseError`) is raised for *every*
failure mode — bad tokens, malformed structure, unexpected end of input, etc.
Carrying the offending line/column lets the CLI print a humane, pinpointed
message instead of a bare stack trace.
"""

from __future__ import annotations


class JSONParseError(Exception):
    """Raised when input is not valid JSON.

    Attributes:
        message: Human-readable description of what went wrong.
        line:    1-based line number where the error was detected.
        column:  1-based column number where the error was detected.
    """

    def __init__(self, message: str, line: int, column: int) -> None:
        self.message = message
        self.line = line
        self.column = column
        super().__init__(f"{message} (line {line}, column {column})")
