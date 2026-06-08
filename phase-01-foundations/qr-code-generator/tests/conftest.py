"""Pytest setup: make the optional zbar decoder importable when present.

pyzbar needs the native zbar shared library. On Linux a normal package install
(`apt install libzbar0`) puts it on the default search path. On macOS (Homebrew)
the dylib lives under a prefix that isn't searched by default, so we add the
common Homebrew lib directories to the dynamic-loader search path *before*
pyzbar is imported. If zbar isn't installed at all, the round-trip tests fall
back to OpenCV or skip — they never hard-fail on a missing decoder.
"""

from __future__ import annotations

import os

_CANDIDATE_LIB_DIRS = ("/opt/homebrew/lib", "/usr/local/lib", "/usr/lib")


def _augment_loader_path() -> None:
    existing = {
        p
        for var in ("DYLD_LIBRARY_PATH", "DYLD_FALLBACK_LIBRARY_PATH", "LD_LIBRARY_PATH")
        for p in os.environ.get(var, "").split(os.pathsep)
        if p
    }
    extra = [d for d in _CANDIDATE_LIB_DIRS if os.path.isdir(d) and d not in existing]
    if not extra:
        return
    for var in ("DYLD_LIBRARY_PATH", "DYLD_FALLBACK_LIBRARY_PATH", "LD_LIBRARY_PATH"):
        current = os.environ.get(var, "")
        parts = [p for p in current.split(os.pathsep) if p] + extra
        os.environ[var] = os.pathsep.join(parts)


_augment_loader_path()
