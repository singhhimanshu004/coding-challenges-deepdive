"""A from-scratch ``yq`` — a YAML processor with a jq-like query language.

The codingchallenges.fyi "build your own yq" challenge. YAML is (roughly) a
superset of JSON, so once a YAML document is loaded into plain Python objects
it can be queried with the *same* lex -> parse -> evaluate skeleton used by the
Phase 1 JSON parser and the Go ``jq`` challenge.

The package is split into four teaching-sized pieces:

* :mod:`yq.loader`  — YAML/JSON text -> Python objects (handles multi-document
  streams, anchors/aliases via PyYAML).
* :mod:`yq.query`   — a jq-like mini-language: lexer -> parser (AST) -> evaluator.
* :mod:`yq.convert` — serialise Python objects back to YAML or JSON.
* :mod:`yq.cli`     — the command-line front end.

Public API::

    from yq import load_documents, compile_query
    docs = load_documents("a:\\n  - 1\\n  - 2\\n")
    prog = compile_query(".a | length")
    [out] = list(prog.run(docs[0]))   # -> 2
"""

from __future__ import annotations

from .convert import to_json, to_yaml
from .errors import YqError, YqQueryError
from .loader import load_documents
from .query import Program, compile_query

__all__ = [
    "load_documents",
    "compile_query",
    "Program",
    "to_json",
    "to_yaml",
    "YqError",
    "YqQueryError",
]
