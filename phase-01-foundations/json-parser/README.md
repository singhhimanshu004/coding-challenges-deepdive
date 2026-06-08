# JSON Parser

> **Phase:** 1 — Foundations: Parsing, Encoding & Data Structures  
> **Difficulty:** 🟢  
> **Recommended Language:** 🟨 Python  
> **Effort Estimate:** M

**Status:** ✅ Done

---

## 🎯 What We're Building

A **JSON parser from scratch** — the same job `json.loads` does, but written by
hand so we actually understand it. Given raw JSON text, our tool turns it into a
native Python value (`dict`, `list`, `str`, `int`, `float`, `bool`, `None`) and,
crucially, **rejects malformed input with a precise, human-readable error** that
points at the offending line and column.

It ships as a small package plus a CLI:

```bash
# Validate + pretty-print a file
python -m jsonparser data.json

# Validate from a pipe; exit code says valid (0) or invalid (1)
echo '{"hello": "world"}' | python -m jsonparser
```

```text
$ echo '{"a": 1,}' | python -m jsonparser
Invalid JSON: Trailing comma in object (line 1, column 9)
$ echo $?
1
```

### Why this is the highest-leverage challenge in the curriculum

JSON looks simple, and that's the point. Its grammar is tiny — six structural
characters, three keywords, strings and numbers — so you can learn the **entire
parsing pipeline end-to-end** without drowning in special cases. The exact same
skeleton you build here (**lex → parse → evaluate**) reappears later in:

- `jq` / `yq` — query languages over structured data,
- the **Calculator** — parsing arithmetic expressions with precedence,
- the **Lisp interpreter** — parsing *and then evaluating* code.

Internalise the parsing mindset once, reuse it a dozen times.

---

## 📚 Core Concepts

### 1. What is parsing?

**Parsing** is turning a flat sequence of characters into a structured,
meaningful representation. Compilers, browsers, config loaders, protocol
decoders — they all parse. The universal trick is to split the work into two
stages so neither stage has to do everything at once:

```
  raw text          tokens                 Python value
 ┌──────────┐  lex ┌──────────────────┐ parse ┌───────────────┐
 │ {"a":[1]}│ ───► │ { "a" : [ 1 ] }  │ ───►  │ {"a": [1]}    │
 └──────────┘      └──────────────────┘       └───────────────┘
   characters         token stream              data structure
```

### 2. Lexing (a.k.a. tokenizing / scanning)

The **lexer** groups characters into the smallest meaningful units — *tokens*.
`{"a":[1]}` becomes the token stream `{`, `"a"`, `:`, `[`, `1`, `]`, `}`. The
lexer handles everything that can be decided by looking at characters *locally*:

- Is `6.022e23` a well-formed number? (Yes.) Is `01`? (No — leading zero.)
- Decoding string escapes: `\n`, `\t`, `\"`, and `\uXXXX` unicode (including
  UTF-16 surrogate pairs for emoji like 😀).

The lexer knows **nothing about grammar** — whether a comma appears in a legal
place is not its job. That separation is what keeps both stages simple.

### 3. Grammars and recursive-descent parsing

JSON's structure is described by a **grammar** (from RFC 8259):

```
json    = value
value   = object | array | string | number | true | false | null
object  = '{' [ member (',' member)* ] '}'
member  = string ':' value
array   = '[' [ value (',' value)* ] ']'
```

Notice it's **recursive**: a `value` can be an `array`, whose elements are
`value`s, which can be `array`s again, forever. A **recursive-descent parser**
mirrors this *directly*: write **one function per grammar rule**, and let the
**call stack** track how deeply you're nested. `_parse_array` calls
`_parse_value`, which calls `_parse_array`... the recursion in the code matches
the recursion in the data.

### 4. Lookahead and LL(1)

Our parser only ever needs to peek at **the next single token** to know which
rule applies — see a `{` and you're parsing an object; see a `[` and it's an
array. Grammars with that property are called **LL(1)**, and they're parseable
with no *backtracking* (we never have to un-read tokens and try again). That's
what makes the code so clean.

### 5. Real-world context

Every API you've ever called returns JSON. Config files (`package.json`,
`tsconfig.json`), log pipelines, message queues, and `localStorage` all speak
JSON. The parser you build here is a miniature of what V8, Python's `json`
module, and `serde_json` do at scale — minus the years of speed tuning.

---

## 🏗️ Architecture & Design

The package is deliberately split into **teaching-sized modules**, each
mapping to one stage of the pipeline:

```
jsonparser/
├── tokens.py    # Token + TokenType: the vocabulary shared by both stages
├── lexer.py     # Stage 1: text  -> tokens
├── parser.py    # Stage 2: tokens -> Python value  (recursive descent)
├── errors.py    # JSONParseError: one error type carrying line/column
├── cli.py       # Command-line front end (file or stdin, exit codes)
└── __main__.py  # enables `python -m jsonparser`
tests/           # pytest: lexer, parser, CLI, edge cases, step fixtures
```

**Data flow:**

```
            ┌─────────┐      ┌──────────┐      ┌─────────────┐
  text ────►│  Lexer  │────► │  Parser  │────► │ dict / list │
            └─────────┘ list └──────────┘ value│ str / int...│
                 │       of        │           └─────────────┘
                 │     tokens      │
                 └──── raises ─────┴──► JSONParseError(msg, line, column)
```

### Key design decisions

- **Two stages, not one.** We *could* parse straight from characters, but
  separating lexing from parsing keeps each piece small and independently
  testable. This is how virtually every production parser is structured.
- **Tokens carry positions.** Every token remembers its `line`/`column`, so any
  error the parser raises can point at the exact spot. Good error messages are
  a feature, not an afterthought.
- **One exception type.** Every failure — bad escape, leading zero, missing
  comma, trailing junk — raises `JSONParseError`. Callers catch one thing.
- **Integers stay integers.** `42` parses to a Python `int` (arbitrary
  precision, so huge numbers keep full precision); `42.0` and `6e2` become
  `float`. This matches `json.loads`.
- **Last-key-wins by default**, with an opt-in strict mode
  (`allow_duplicate_keys=False`) that rejects duplicates — RFC 8259 permits
  either, and we let the caller choose.

### Why recursive descent instead of a table-driven parser?

| | Recursive descent (chosen) | Table-driven (LR/LALR, e.g. yacc) |
|---|---|---|
| **Readability** | Code *looks like* the grammar — one function per rule | Opaque state-transition tables, usually generated |
| **Error messages** | Easy and precise (you're in a named function) | Harder; you only know a "parse state number" |
| **Hand-writability** | Trivial for LL(1) grammars like JSON | Practically requires a generator tool |
| **Power** | LL(k); struggles with left-recursion | Handles a larger class of grammars |
| **Stack usage** | Bounded by host language recursion depth | Uses an explicit heap-allocated stack |

For a small, non-left-recursive, LL(1) grammar like JSON, recursive descent
wins on every axis that matters for *learning*: the code is self-documenting and
the errors are excellent. The one real cost is the stack-depth limit (below).

### The recursive-descent trade-off: stack depth

Because nesting depth in the data becomes recursion depth in the parser, a
pathologically deep document (`[[[[ ... ]]]]`, thousands deep) can hit CPython's
default recursion limit (~1000 frames). Production parsers either raise the
limit (`sys.setrecursionlimit`) or rewrite the recursion into an explicit loop +
heap stack. We keep the clean recursive form here because clarity is the goal;
the limit is far beyond any realistic JSON document.

---

## 🔨 Step-by-Step Implementation

Built incrementally, mirroring the codingchallenges.fyi steps.

### Step 0 — Vocabulary (`tokens.py`)
Define `TokenType` (the lexical categories) and a `Token` dataclass holding a
type, a decoded value, and a source position. Both stages share this vocabulary.

### Step 1 — Parse `{}` (empty object)
The smallest valid document. Get the lexer emitting `{`, `}`, `EOF`, and the
parser turning that into `{}`. Reject `` (empty) and `{` (unterminated).

### Step 2 — String keys and values
Teach the lexer to scan `"..."` strings, resolving escapes. Teach the parser the
`member = string ':' value` rule and the comma-separated member loop. This is
where we **reject trailing commas** (`{"a":1,}`) — a comma must be followed by
another member.

### Step 3 — All the scalar value types
Add numbers (the trickiest lexing: signs, fractions, exponents, and rejecting
leading zeros) and the `true` / `false` / `null` keywords. Now `_parse_value`
dispatches on the next token to the right handler.

### Step 4 — Nesting (objects & arrays inside values)
Add `_parse_array` and let `_parse_value` recurse into both arrays and objects.
This is the moment recursive descent earns its name — and suddenly arbitrarily
nested JSON Just Works.

### Step 5 — Polish: errors, edge cases, CLI
Line/column in every error; reject trailing junk after the top-level value
(`{} {}` is two documents, not one); the unicode surrogate-pair path; and the
`cli.py` front end with file/stdin input and meaningful exit codes.

The number scanner deserves a close look — it's a hand-coded version of this
grammar diagram:

```
        ┌───┐        ┌──────────────┐   ┌────────────────────┐
 ──►(-)?─┤int├──(.digits)?──┤ frac (opt)│──(e/E (+/-)? digits)?─┤ exp (opt) │──►
        └───┘        └──────────────┘   └────────────────────┘
  int = '0'  |  digit1-9 digit*      ← this rule is why '01' is illegal
```

---

## 🧪 Testing Strategy

Tests live in `tests/` and run with **pytest**. The strategy has four layers:

1. **Unit-test the lexer** (`test_lexer.py`) — every token type, all escape
   forms, surrogate pairs, and each malformed-number shape (`01`, `1.`, `.5`,
   `1e`, bare `-`).
2. **Unit-test the parser** (`test_parser.py`) — valid documents *and* a battery
   of invalid ones. The killer technique: for every input we also assert our
   **accept/reject verdict agrees with the standard library's `json`** module.
   `json` is the ground truth for "is this valid JSON?", so any disagreement is
   a bug in our parser.
3. **Step fixtures** (`test_steps.py`) — escalating valid/invalid cases in the
   spirit of the codingchallenges.fyi `tests/step1…step4` files.
4. **CLI tests** (`test_cli.py`) — file input, stdin, exit codes (0/1/2),
   `--quiet`, and the strict `--no-duplicate-keys` mode.

Edge cases explicitly covered: empty input, whitespace-only, trailing commas,
unterminated strings/objects, missing commas/colons, unquoted & single-quoted
keys, leading zeros, deep nesting, duplicate keys, huge integers, trailing junk,
and exact line/column reporting.

### How to run it

```bash
# From this directory (phase-01-foundations/json-parser/)

# 1. Create a virtualenv and install pytest
python3 -m venv .venv
source .venv/bin/activate
pip install pytest

# 2. Run the parser on a file or via stdin
python -m jsonparser path/to/file.json
echo '{"ok": true}' | python -m jsonparser
python -m jsonparser --quiet file.json   # validator only (no output)

# 3. Run the tests
python -m pytest -q
```

All **110 tests pass**.

---

## 💡 Key Takeaways

- **Parsing = lex then parse.** Splitting "characters → tokens" from "tokens →
  structure" is the single most important idea; it keeps every parser tractable.
- **Recursive descent makes a grammar executable.** One function per rule, and
  the call stack does the nesting bookkeeping for free. The code reads like the
  spec.
- **A grammar is a checklist.** Most "JSON validation" is just faithfully
  implementing each production — and the subtle rules (no leading zeros, no
  trailing commas, escaped control chars) are exactly where bugs hide.
- **Error messages are a feature.** Threading line/column through tokens turns a
  useless "invalid input" into "Trailing comma in object (line 1, column 9)".
- **Know your trade-offs.** Recursive descent is the clearest tool for LL(1)
  grammars; its cost is host-language stack depth, which you mitigate with an
  explicit stack only when you must.
- **This skeleton is reusable.** `jq`, `yq`, the Calculator, and the Lisp
  interpreter are the *same* lex→parse pattern with a bigger grammar (and an
  evaluation stage bolted on the end).

---

## 📖 Further Reading

- **The challenge:** [Build Your Own JSON Parser — codingchallenges.fyi](https://codingchallenges.fyi/challenges/challenge-json-parser)
- **The spec:** [RFC 8259 — The JSON Data Interchange Format](https://www.rfc-editor.org/rfc/rfc8259) and the interactive railroad diagrams at [json.org](https://www.json.org/json-en.html)
- **Torture tests:** [JSONTestSuite](https://github.com/nst/JSONTestSuite) — Nicolas Seriot's exhaustive valid/invalid corpus (and the famous "Parsing JSON is a Minefield" article)
- **Recursive descent, deeper:** Bob Nystrom, *[Crafting Interpreters](https://craftinginterpreters.com/)* — the canonical, free, beautifully written intro to scanners and recursive-descent parsers
- **The reference implementation:** CPython's [`json` module source](https://github.com/python/cpython/tree/main/Lib/json) — see how the real thing handles the same problems
