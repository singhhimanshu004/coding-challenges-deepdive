# yq (YAML processor)

> **Phase:** 3 — Advanced CLI & Orchestration
> **Difficulty:** 🔵
> **Recommended Language:** 🟨 Python
> **Effort Estimate:** M

**Status:** ✅ Done

---

## 🎯 What We're Building

A from-scratch **`yq`** — a command-line YAML processor, modelled on
[codingchallenges.fyi "Build Your Own yq"](https://codingchallenges.fyi/challenges/challenge-yq).
It does three things:

1. **Loads YAML** (and JSON, which is valid YAML) into plain Python objects,
   including multi-document streams and anchor/alias references.
2. **Queries and transforms** the data with a small **jq-like path language**:
   identity `.`, field access `.foo.bar`, indexing `.[0]`, iteration `.[]`,
   the pipe `|`, and the builtins `length` and `keys`.
3. **Converts** between formats: YAML → JSON (`-o json`) and JSON → YAML,
   pretty or compact, reading from a file or stdin, with sensible exit codes.

```console
$ python -m yq '.production.region' -o json sample.yaml
"us-east-1"

$ python -m yq '.ports[]' sample.yaml
--- 80
--- 443

$ echo '{"a":[1,2,3]}' | python -m yq '.a | length'
3
```

The headline idea: **`yq` is `jq` for YAML**. YAML is (roughly) a superset of
JSON, so once a document is parsed into ordinary dicts/lists/scalars, the query
engine is the *same* `lex → parse → evaluate` skeleton you built for the Phase 1
JSON parser and the Go `jq` challenge. We deliberately **build the query engine
by hand** rather than call an existing `jq` — that interpreter is the lesson.

> 🐍 You're fully at home in Python, so this README spends its energy on the two
> conceptually rich parts: **the YAML data model** and **the path-language
> interpreter**. The YAML *parsing* itself we delegate to PyYAML — re-tokenising
> YAML's enormous grammar would teach little and distract from the core.

---

## 📚 Core Concepts

### 1. The YAML data model

YAML ("YAML Ain't Markup Language") is a human-friendly data serialisation
format. Strip away the syntax and every YAML document is a tree built from just
three kinds of node:

| Node type    | YAML example            | Python type                       |
|--------------|-------------------------|-----------------------------------|
| **Scalar**   | `42`, `true`, `hello`   | `int` / `bool` / `str` / `float` / `None` |
| **Sequence** | `- a` / `- b`           | `list`                            |
| **Mapping**  | `key: value`            | `dict`                            |

That's the whole model. A real document is just these nested arbitrarily:

```yaml
service: web          # mapping: str -> str (scalar)
replicas: 3           # mapping: str -> int (scalar)
ports:                # mapping: str -> sequence
  - 80
  - 443
metadata:             # mapping: str -> mapping
  team: platform
```

**Scalars and implicit typing.** Unquoted scalars are *type-inferred*: `3` is an
int, `3.14` a float, `true`/`false` a bool, `null`/`~`/empty is null, and
anything else is a string. Quote a scalar (`"3"`, `'true'`) to force a string.
This is a common foot-gun (the famous "Norway problem": `NO` → `False`).

**Block style vs flow style.** YAML offers two ways to write collections:

```yaml
# Block style (indentation-based, the readable default)
ports:
  - 80
  - 443

# Flow style (inline, JSON-like)
ports: [80, 443]
```

They parse to the *identical* data. Flow style is where YAML visibly overlaps
JSON — and indeed `{"ports": [80, 443]}` is legal YAML.

### 2. YAML vs JSON — "a superset, roughly"

JSON is (very nearly) a strict subset of YAML 1.2. Anything valid JSON is valid
YAML and loads to the same structure. But YAML adds features JSON has no syntax
for:

| Feature                 | JSON | YAML |
|-------------------------|:----:|:----:|
| Objects / arrays / scalars | ✅ | ✅ |
| Comments (`# ...`)      | ❌ | ✅ |
| Anchors & aliases       | ❌ | ✅ |
| Multiple documents      | ❌ | ✅ |
| Block vs flow style     | ❌ | ✅ |
| Unquoted strings        | ❌ | ✅ |

The practical consequence drives this whole tool's architecture: **once you load
YAML into Python objects, the extra YAML features have been "compiled away."**
Anchors are resolved to shared objects, comments are dropped, a multi-doc stream
becomes a list of documents. After loading, YAML→JSON is just "serialise these
Python objects with `json.dumps`," and the query engine never has to know the
data came from YAML at all.

### 3. Anchors (`&`) and aliases (`*`)

YAML lets you name a node once and reuse it, avoiding repetition (DRY config):

```yaml
defaults: &defaults      # &defaults declares an anchor
  retries: 5
  timeout: 30
production:
  <<: *defaults          # *defaults aliases it; << merges its keys in
  region: us-east-1
staging:
  <<: *defaults
  region: us-west-2
```

`&name` *declares* an anchor on a node; `*name` is an *alias* that refers back to
it; `<<` is the **merge key** that splices a referenced mapping's keys into the
current one. The loader resolves all of this, so `production` simply comes out as
`{retries: 5, timeout: 30, region: us-east-1}` — a plain dict. (Aliases to the
same collection become the *same* Python object, i.e. shared by reference.)

### 4. Multi-document streams

A single YAML file can hold several documents separated by `---` (and optionally
ended by `...`). This is common in Kubernetes manifests:

```yaml
kind: Service
name: web
---
kind: Deployment
name: web
replicas: 3
```

We load this with `yaml.safe_load_all`, which yields **one Python object per
document**, so a stream becomes a `list` of documents. The query runs against
every document in turn.

### 5. Safe loading

We use `yaml.safe_load` / `safe_load_all`, never the unrestricted loader. The
default loader can construct arbitrary Python objects via tags like
`!!python/object/apply:os.system`, which is a remote-code-execution hazard when
parsing untrusted input. The safe loader only ever builds standard
scalar/list/dict types — exactly what we want, and a subset that maps cleanly to
JSON.

---

## 🏗️ Architecture & Design

The package mirrors the data flow, one module per stage:

```
                 ┌──────────────────────── yq/cli.py ─────────────────────────┐
                 │  parse argv · read file/stdin · pick output format · exits  │
                 └───────┬─────────────────────────────────────────┬──────────┘
                         │ text                                     │ Python objects
                         ▼                                          ▼
         ┌──────── yq/loader.py ────────┐            ┌──────── yq/convert.py ────────┐
         │ YAML/JSON text → [documents] │            │ Python objects → YAML / JSON  │
         │ (PyYAML safe_load_all)       │            │ (yaml.safe_dump / json.dumps) │
         └──────────────┬───────────────┘            └───────────────────────────────┘
                        │ documents (dicts/lists/scalars)
                        ▼
         ┌──────────────────────── yq/query.py ───────────────────────────────┐
         │  Lexer:     query text  →  tokens                                   │
         │  Parser:    tokens      →  AST  (Identity, Field, Index, Iterate,   │
         │                                  Pipe, Path, Builtin)               │
         │  Evaluator: AST.eval(value) → a *stream* of output values           │
         └─────────────────────────────────────────────────────────────────────┘
```

| Module        | Responsibility                                                    |
|---------------|-------------------------------------------------------------------|
| `loader.py`   | Parse YAML/JSON into Python objects; handle multi-doc & errors.    |
| `query.py`    | The teaching core: lexer + parser + evaluator for the path language. |
| `convert.py`  | Serialise Python objects back to YAML or JSON (pretty/compact).    |
| `cli.py`      | Argument parsing, I/O, output dispatch, exit codes.               |
| `errors.py`   | `YqError` hierarchy that maps cleanly onto exit codes.            |

### The key design idea: the value **stream**

Every AST node, when evaluated against **one** input value, produces a **stream**
(a Python generator) of **zero or more** output values. This single abstraction
makes the whole language compose:

- Identity `.` yields its input unchanged → a stream of one.
- `.foo` yields the field's value → a stream of one (or `null`).
- `.[]` **explodes** a list/dict into a stream of *many* values — this is the
  only node that fans out.
- `|` (pipe) feeds *every* value from the left stream into the right node and
  concatenates the results — this is the only node that composes streams.

So `.users[] | .name` reads exactly as it looks: explode `users` into a stream of
user objects, then map each to its `.name`. This is precisely how `jq` models
computation, and copying that model is what makes the engine small yet powerful.

---

## 🔨 Step-by-Step Implementation

### Step 1 — Load YAML into Python (`loader.py`)

`load_documents(text)` calls `yaml.safe_load_all` and returns a `list` of
documents (one element for a single document, `[None]` for empty input so
callers always have something to query). PyYAML errors are caught and re-raised
as `YqLoadError` with a tidy `line/column` message.

### Step 2 — Lex the query (`query.py`, Stage 1)

A hand-written scanner turns the query string into a flat token list. The token
kinds are tiny: `DOT  PIPE  LBRACKET  RBRACKET  INT  STRING  IDENT  EOF`. It
handles negative integers (`-1`), quoted bracket keys (`.["a b"]`) with escape
sequences, and identifiers for builtins/fields.

### Step 3 — Parse into an AST (`query.py`, Stage 2)

A recursive-descent parser implements this grammar:

```
program  := pipeline EOF
pipeline := primary ( '|' primary )*
primary  := builtin | path
builtin  := 'length' | 'keys'
path     := '.' step*
step     := '.' IDENT  |  '[' INT ']'  |  '[' STRING ']'  |  '[' ']'
```

Each rule produces a node: `Identity`, `Field(name)`, `Index(i)`, `Iterate()`,
`Pipe(left, right)`, `Path(steps)`, or `Builtin(name)`. A bare path with no
steps collapses to `Identity`; a single step is returned directly; multiple
steps become a `Path` that threads the stream through each step in turn.

### Step 4 — Evaluate (`query.py`, Stage 3)

Every node implements `eval(value) -> Iterator`. The semantics follow `jq`:

- `Field` on a `dict` returns the value (or `None` if missing); on `null`
  returns `null`; on anything else raises `YqQueryError`.
- `Index` supports negative indices and returns `null` when out of range.
- `Iterate` fans a list (elements) or dict (values) into the stream; iterating a
  scalar is an error.
- `Pipe` threads the left stream through the right node.
- `length`: `null`→0, string→character count, list/dict→element count,
  number→absolute value. `keys`: sorted keys of a dict, or `[0..n-1]` indices of
  a list.

### Step 5 — Convert output (`convert.py`)

`to_json` uses `json.dumps` (pretty with `indent`, or `compact` with tight
separators; `ensure_ascii=False` keeps Unicode readable). `to_yaml` uses
`yaml.safe_dump` with `default_flow_style` toggling block vs flow style. Because
both writers consume the *same* Python objects, format conversion is just
"choose the writer."

### Step 6 — Wire up the CLI (`cli.py`)

`yq [options] [FILTER] [FILE]` — the filter is the first positional (default
`.`), the file is optional (stdin or `-` otherwise). The query is compiled
*first* so a typo fails fast. Results from all documents are gathered and emitted
in the chosen format. Exit codes:

| Code | Meaning                                                      |
|:----:|-------------------------------------------------------------|
| `0`  | success                                                     |
| `1`  | invalid input document, or a query error at evaluation time |
| `2`  | usage / IO problem (bad flags, file not found, bad query)   |

---

## 🧪 Testing Strategy

`pytest` suite (`tests/`), 54 cases across four files:

- **`test_loader.py`** — scalars→Python types, sequences→lists, mappings→dicts,
  JSON-as-YAML, block≡flow equivalence, anchor/alias resolution, multi-document
  streams, empty input, malformed-input errors.
- **`test_query.py`** — every query feature: identity, field, nested field,
  missing field → null, field-on-null, bracket string keys, indexing (incl.
  negative & out-of-range), iteration over lists *and* objects, pipes, `length`
  (string/list/null/number), `keys` (object & array), plus parse-error cases.
- **`test_convert.py`** — YAML↔JSON round-trips, compact vs pretty JSON, block vs
  flow YAML, multi-document output separators, Unicode preservation.
- **`test_cli.py`** — end-to-end through `main()` with injected stdin/stdout:
  stdin input, JSON output, multi-output iteration, JSON input, multi-document
  processing, anchor/alias queries, and the three exit codes.

```console
$ python -m venv .venv && source .venv/bin/activate
$ pip install -r requirements.txt pytest
$ python -m pytest -q
54 passed
```

### Try it on the sample

```console
$ python -m yq '.' sample.yaml                 # echo as YAML
$ python -m yq '.metadata.team' sample.yaml    # → platform
$ python -m yq '.ports | length' sample.yaml   # → 2
$ python -m yq 'keys' sample.yaml              # sorted top-level keys
$ python -m yq '.production' -o json sample.yaml   # merged anchor → JSON
$ python -m yq '.' -o json -c sample.yaml      # whole file → compact JSON
$ echo '{"a":[1,2],"b":"x"}' | python -m yq '.'    # JSON in → YAML out
```

---

## 💡 Key Takeaways

- **YAML is a tree of three node kinds** — scalar, sequence, mapping — that map
  one-to-one onto Python's `str/int/bool/None`, `list`, and `dict`. Internalise
  that and YAML stops being mysterious.
- **"YAML is a JSON superset" is an architectural lever, not trivia.** Loading
  normalises everything to plain Python objects, so the query engine, JSON
  output, and YAML output are all format-agnostic. Conversion is "pick a writer."
- **Anchors/aliases and multi-document streams are resolved at load time**, so
  the rest of the program is blissfully unaware of them.
- **The value-stream model is the soul of jq/yq.** Nodes map one input to a
  stream of outputs; `.[]` fans out, `|` composes. From those two ideas the
  entire query language falls out — and it's the *same* `lex→parse→evaluate`
  skeleton as the JSON parser.
- **Don't re-implement the boring part.** Delegating YAML tokenisation to PyYAML
  (a hardened, spec-compliant parser) keeps the focus on the genuinely
  instructive code: the data model and the interpreter. Use `safe_load` — the
  default loader is an RCE risk on untrusted input.

---

## 📖 Further Reading

- [codingchallenges.fyi — Build Your Own yq](https://codingchallenges.fyi/challenges/challenge-yq)
- [YAML 1.2.2 specification](https://yaml.org/spec/1.2.2/)
- [`jq` manual](https://jqlang.github.io/jq/manual/) — the query language we mimic
- [mikefarah/yq](https://github.com/mikefarah/yq) and [kislyuk/yq](https://github.com/kislyuk/yq) — two real-world `yq` tools
- [PyYAML documentation](https://pyyaml.org/wiki/PyYAMLDocumentation)
- Phase 1 **JSON Parser** in this repo — the same lexer/parser/evaluator skeleton
