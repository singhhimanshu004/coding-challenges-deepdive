"""YAML/JSON loading — text becomes plain Python objects.

This is the *parsing infrastructure* layer. We lean on PyYAML to turn YAML
text into Python data; the teaching value here is the **data model**, not
re-implementing a YAML tokenizer (YAML's grammar is famously large).

The YAML data model maps cleanly onto Python (and therefore JSON):

* a **scalar**   -> ``str`` / ``int`` / ``float`` / ``bool`` / ``None``
* a **sequence** -> ``list``
* a **mapping**  -> ``dict``

Two YAML features that JSON lacks are resolved *during loading* so the rest of
the program never has to think about them:

* **Anchors (`&name`) and aliases (`*name`)** let a node be defined once and
  referenced elsewhere. PyYAML resolves them into shared Python objects, so a
  document with an alias simply yields equal (and, for collections, identical)
  values at each reference site.
* **Multi-document streams** separate documents with ``---``. We use
  :func:`yaml.safe_load_all`, which yields one Python object per document, so a
  stream becomes a ``list`` of documents.

We deliberately use the **safe** loader: it only constructs standard Python
types and never executes arbitrary tags like ``!!python/object``. Since JSON is
a subset of YAML, this same loader reads JSON input for free.
"""

from __future__ import annotations

from typing import Any, List

import yaml

from .errors import YqLoadError

__all__ = ["load_documents"]


def load_documents(text: str) -> List[Any]:
    """Parse *text* as a YAML (or JSON) stream and return a list of documents.

    A single-document input returns a one-element list. An empty stream
    returns ``[None]`` so callers always have at least one document to act on,
    mirroring how ``yq``/``jq`` treat empty input as ``null``.

    Raises :class:`YqLoadError` on malformed input.
    """
    try:
        documents = list(yaml.safe_load_all(text))
    except yaml.YAMLError as exc:
        raise YqLoadError(_format_yaml_error(exc)) from exc

    if not documents:
        return [None]
    return documents


def _format_yaml_error(exc: yaml.YAMLError) -> str:
    """Produce a compact, human-friendly message from a PyYAML error."""
    mark = getattr(exc, "problem_mark", None)
    problem = getattr(exc, "problem", None)
    if mark is not None and problem:
        return f"{problem} (line {mark.line + 1}, column {mark.column + 1})"
    return str(exc).strip() or "invalid YAML"
