"""Exception types for the yq package.

A small hierarchy keeps the CLI honest about exit codes:

* :class:`YqError`       — base class; anything that should map to a failure.
* :class:`YqLoadError`   — the input was not valid YAML/JSON.
* :class:`YqQueryError`  — the query expression was malformed, or could not be
  evaluated against the data (e.g. indexing a string).
"""

from __future__ import annotations


class YqError(Exception):
    """Base class for all yq failures."""


class YqLoadError(YqError):
    """Raised when the input document cannot be parsed as YAML/JSON."""


class YqQueryError(YqError):
    """Raised for a malformed query or an illegal operation at evaluation time."""
