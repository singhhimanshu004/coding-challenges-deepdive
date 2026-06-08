"""Step-based tests in the spirit of the codingchallenges.fyi JSON parser.

The challenge ships escalating ``tests/step*/valid.json`` and ``invalid.json``
fixtures. We embed equivalent fixtures here and assert that every ``valid``
sample parses and every ``invalid`` sample is rejected — exactly the
acceptance gate the original challenge uses.
"""

from __future__ import annotations

import pytest

from jsonparser import JSONParseError, parse

# Step 1: the simplest objects — empty and non-empty.
STEP1_VALID = ["{}"]
STEP1_INVALID = ["", "{"]

# Step 2: string keys and string values, multiple members.
STEP2_VALID = ['{"key": "value"}', '{"key": "value", "key2": "value"}']
STEP2_INVALID = ['{"key": "value",}', '{"key": "value" "key2": "value"}']

# Step 3: the full set of value types.
STEP3_VALID = [
    '{"key1": true, "key2": false, "key3": null, "key4": "value", "key5": 101}'
]
STEP3_INVALID = ['{"key1": true, "key2": False, "key3": null}']  # capital-F

# Step 4: nested objects and arrays.
STEP4_VALID = [
    '{"key": "value", "key-n": 101, "key-o": {}, "key-l": []}',
    '{"key": "value", "key-n": 101, "key-o": {"inner": {}}, "key-l": ["list"]}',
]
STEP4_INVALID = ['{"key": "value", "key-n": 101, "key-o": {"inner": {}, "key-l": [\'list\']}}']


@pytest.mark.parametrize(
    "doc",
    STEP1_VALID + STEP2_VALID + STEP3_VALID + STEP4_VALID,
)
def test_step_valid(doc):
    parse(doc)  # must not raise


@pytest.mark.parametrize(
    "doc",
    STEP1_INVALID + STEP2_INVALID + STEP3_INVALID + STEP4_INVALID,
)
def test_step_invalid(doc):
    with pytest.raises(JSONParseError):
        parse(doc)
