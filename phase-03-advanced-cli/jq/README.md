# jq (JSON processor)

> **Phase:** 3 — Advanced CLI & Orchestration
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Done

> 🐍 **New to Go?** This README is written for a Python developer learning Go.
> Read the project primer first: [`docs/go-quickstart.md`](../../docs/go-quickstart.md).
> It maps every Go concept used here (slices, structs, interfaces, type
> switches, `iota` enums, `any`/`interface{}`) back to the Python you know.

---

## 🎯 What We're Building

A from-scratch rebuild of [`jq`](https://jqlang.github.io/jq/), the "sed for
JSON", following the
[codingchallenges.fyi "build your own jq"](https://codingchallenges.fyi/challenges/challenge-jq/)
spec. You hand it a tiny **filter program** and some JSON, and it transforms one
into the other:

```console
$ echo '{"name":"Ada","age":36}' | ./jq '.name'
"Ada"

$ ./jq '.people[] | select(.age > 30) | .name' team.json
"Bob"

$ ./jq -c 'map(.name)' people.json
["Bob","Cy"]
```

The headline idea — and the reason this challenge exists — is that **`.foo`,
`|`, `select(...)` and friends are a real little programming language.** So jq is
not "a JSON tool with some flags". It is a *compiler + interpreter* for an
expression language whose values happen to be JSON. That is exactly the
[Phase-1 JSON parser](../../phase-01-foundations/json-parser/) skill — `lex →
parse → evaluate` — applied twice: once to the JSON **data**, and once to the
**filter program**.

### Supported filter language

| Filter | Meaning | Example |
| --- | --- | --- |
| `.` | identity (input unchanged) | `.` |
| `.foo` | object field access | `.name` |
| `.foo.bar` | nested access | `.address.city` |
| `.foo?` | optional — suppress errors | `.maybe?` |
| `.[0]`, `.[-1]` | array index (negatives from end) | `.langs[0]` |
| `.["k"]` | dynamic key index | `.["my key"]` |
| `.[]` | iterate array elements / object values | `.people[]` |
| `\|` | pipe — feed left's outputs into right | `.a \| .b` |
| `,` | run both, concatenate output streams | `.a, .b` |
| `[ … ]` | array construction (collect a stream) | `[.[] \| .name]` |
| `( … )` | grouping | `(.a + .b)` |
| `==  !=  <  >  <=  >=` | comparisons | `.age > 30` |
| `+  -  *  /` | arithmetic (`+` also joins strings/arrays/objects) | `.x * 2` |
| `length` | size of string/array/object/number | `.langs \| length` |
| `keys` | sorted keys (object) / indices (array) | `keys` |
| `values` | the values of an object / elements of an array | `values` |
| `has(k)` | membership test | `has("age")` |
| `select(f)` | keep input when `f` is truthy | `select(.age > 30)` |
| `map(f)` | apply `f` to every array element | `map(.name)` |

### CLI flags

| Flag | Long form | Meaning |
| --- | --- | --- |
| `-c` | `--compact-output` | one line per result, no pretty indenting |
| `-r` | `--raw-output` | print string results without the quotes |
| `-S` | `--sort-keys` | emit object keys sorted |
| `-C` | `--color-output` | ANSI colours |
| `-M` | `--monochrome-output` | force colour off |

Input comes from file arguments (`./jq '.' a.json b.json`) or from **stdin**
when no files are given. Exit codes: `0` success, `1` runtime error (e.g.
indexing a number), `2` usage error (bad flag, bad filter, unreadable file, or
invalid JSON).

---

## 📚 Core Concepts

### 1. JSON values become a Go data model

Before we can filter JSON we must hold it in memory. We model the seven JSON
types with Go's `any` (the new spelling of `interface{}` — a box that can hold
any type):

| JSON | Go |
| --- | --- |
| `null` | `nil` |
| `true`/`false` | `bool` |
| number | `float64` |
| string | `string` |
| array | `[]any` |
| object | `*Object` (a small **insertion-ordered** map) |

> 🐍 In Python `json.loads` hands you `dict/list/str/int/float/bool/None`. Same
> idea — we just build the loader ourselves.

**Why a custom `*Object` instead of Go's built-in `map[string]any`?** Because Go
maps deliberately **randomise** iteration order, but real jq *preserves the key
order of the input document*. Our `Object` keeps a `keys []string` order list
beside the `map`, so output matches jq byte-for-byte. (`collections.OrderedDict`,
if you like.)

### 2. The filter is a language — so we build a compiler

The deep idea: **a jq filter is a function from one input value to a *stream* of
output values.** `.` returns one value; `.[]` returns many; `select(false)`
returns *zero*. Everything composes from that.

We process the filter text in the same three stages as the JSON parser:

```
filter text  ──lex──▶  tokens  ──parse──▶  AST  ──evaluate──▶  output stream
".[]|.x"               [. [ ] | . x]      tree            value, value, …
```

### 3. Tree-walking evaluation

The evaluator is a single recursive function:

```go
func eval(node Node, input any) ([]any, error)
```

It takes an AST node and the current input value, and returns the stream of
results (we model "a stream" as a `[]any` slice). To run a pipe `A | B`, we
`eval(A, input)` and then `eval(B, x)` for **each** `x` A produced. That single
rule is what makes `.[] | select(...) | .name` work.

---

## 🏗️ Architecture & Design

Clean file split — one responsibility each (the same flat, no-`internal/` layout
used by the Phase-2 Go tools):

| File | Responsibility | Pipeline stage |
| --- | --- | --- |
| `jsonval.go` | JSON value model (`*Object`) + hand-rolled JSON parser | parse **data** |
| `lexer.go` | filter tokenizer (text → tokens) | lex **program** |
| `parser.go` | filter grammar → AST node types | parse **program** |
| `eval.go` | tree-walking evaluator + builtins + value ordering | evaluate |
| `output.go` | encode values back to JSON (pretty / compact / colour) | print |
| `main.go` | CLI: flags, stdin/file input, `run()` orchestration | glue |

### The AST, drawn

The filter `.people[] | select(.age > 30) | .name` parses to this tree (pipes
are left-associative, so they nest to the left):

```
              Pipe
             /    \
          Pipe     Field(".name")
         /    \
     Pipe      Call(select)
    /    \          │
 Field    Iterate   └─ arg: Binary(>)
("people")              /        \
                   Field(".age")  Literal(30)
```

Read it left-to-right by following the leftmost branch down: get `.people`,
iterate `[]`, keep the ones where `.age > 30`, then take `.name`.

> 🐍 This is literally what Python's `ast.parse` produces for source code — a
> tree of typed nodes. Each node here is a tiny Go struct (`Field`, `Pipe`,
> `Call`, …) implementing a marker `Node` interface, the way every Python AST
> class extends `ast.AST`.

### The filter grammar (lowest precedence first)

```
pipe     := comma ( '|' comma )*
comma    := compare ( ',' compare )*
compare  := additive ( ('=='|'!='|'<'|'>'|'<='|'>=') additive )?
additive := multiply ( ('+'|'-') multiply )*
multiply := postfix  ( ('*'|'/') postfix )*
postfix  := primary ( '.'IDENT | '['expr?']' | '?' )*
primary  := '.' IDENT?            # field access or bare identity
          | '[' pipe? ']'          # array construction
          | '(' pipe ')'           # grouping
          | NUMBER | STRING        # literals
          | IDENT ( '(' args ')' )? # builtin / function call
```

Each rule is one method in `parser.go`. Rules listed **earlier** bind **looser**,
so `|` is the weakest operator and a path suffix like `.foo` is the tightest —
exactly jq's precedence. This is *recursive-descent parsing*: the grammar's shape
*is* the call graph.

### One neat trick: paths are just pipes

`.foo.bar` could need special "path" machinery. Instead, the parser rewrites
every chained suffix into a `Pipe`: `.foo.bar` becomes `Pipe(Field("foo"),
Field("bar"))`, and `.items[]` becomes `Pipe(Field("items"), Iterate)`. So the
evaluator only ever has to understand one composition rule ("apply the right side
to each output of the left side") and field access falls out for free.

---

## 🔨 Step-by-Step Implementation

1. **Value model + JSON parser (`jsonval.go`).** Recursive descent over the raw
   text: `parseValue` peeks one byte and dispatches to `parseObject` /
   `parseArray` / `parseString` / `parseNumber` / literals. Handles `\uXXXX`
   escapes and UTF-16 surrogate pairs. `ParseJSONStream` loops to read a *stream*
   of whitespace-separated values (so `echo '1 2 3' | jq .` sees three inputs).
2. **Filter lexer (`lexer.go`).** Scan the filter text into typed `token`s,
   collapsing whitespace and multi-character operators (`>=`, `==`, `!=`). A `-`
   becomes a negative-number literal only when a digit follows; otherwise it is
   the minus operator.
3. **Filter parser (`parser.go`).** One method per grammar rule, building the AST.
   `parsePostfix` is where `.foo`, `[idx]`, `[]` and `?` get chained into pipes.
4. **Evaluator (`eval.go`).** The `eval(node, input) ([]any, error)` type switch.
   Pipe feeds streams; Comma concatenates; `[ … ]` collects a whole stream into
   one array; `Try` swallows errors into an empty stream. Builtins live here too,
   plus jq's cross-type value ordering (`null < false < true < numbers < strings
   < arrays < objects`) used by comparisons.
5. **Output (`output.go`).** A recursive encoder: pretty (2-space indent, like
   jq) or compact `-c`, optional `-S` sorted keys, optional `-C` ANSI colour, and
   `-r` raw strings.
6. **CLI (`main.go`).** Thin `main()` → `run(args, stdin, stdout, stderr) int`.
   Hand-rolled flag parser (bundled short flags, `--` terminator), reads files or
   stdin, compiles the filter once, applies it to every input value.

---

## 🧪 Testing Strategy

`jq_test.go` covers the whole pipeline with table-driven Go tests:

- **JSON parsing:** scalars, nesting, key-order preservation, `\u` escapes, and
  malformed-input errors.
- **Every filter feature:** identity, field, nested field, missing-field-is-null,
  optional `?`, array index (incl. negative + out-of-range), `.[]` over arrays
  *and* objects, pipe, comma, array construction.
- **Builtins:** `length`, `keys`, `values`, `has`, `select`, `map` (including
  `map(. * 2)`).
- **Comparisons & arithmetic**, including cross-type ordering and divide-by-zero.
- **Composed real filters**, e.g. `.people[] | select(.age > 30) | .name`.
- **Parser error cases** (`.[`, `| .a`, `select(`, …).
- **Encoder:** compact vs pretty indent, `-S` sort-keys, number formatting.
- **End-to-end `run()`:** stdin, `-r`, multiple inputs, and each exit code
  (`0/1/2`) via in-memory buffers — no subprocess, no temp files.

```console
$ go test ./...
ok   jq
$ go vet ./...
```

Output was also **differentially checked against the real `jq`** (`brew install
jq`) for `.`, `.name`, `.address.city`, `.langs[0]`, `select`, `map(.name)` and
`keys` — all identical.

---

## 💡 Key Takeaways

- **A filter language is a real language.** The single biggest lesson: `jq` is a
  `lex → parse → evaluate` interpreter, the same skeleton as the JSON parser, the
  Calculator and the Lisp interpreter later in the curriculum.
- **Streams, not single values.** Modelling every expression as
  `input → []output` is what makes pipes, commas, `select` (zero outputs) and
  `.[]` (many outputs) all fall out of *one* composition rule.
- **Paths reduce to pipes.** Rewriting `.a.b` into `Pipe(Field a, Field b)` keeps
  the evaluator tiny.
- **Grammar precedence = call-graph order.** In recursive descent, the order you
  call rules *is* operator precedence.
- **Go idioms surfaced for a Python dev:** `any`/`interface{}` as a tagged value,
  `iota` enums for token types, the type switch as a visitor, the comma-ok idiom
  (`v, ok := m[k]`), and the thin-`main`/testable-`run` split reused from Phase 2.

### Design decisions & honest divergences

- **Hand-rolled JSON parser (not `encoding/json`).** *Chosen for the learning
  value* — it reuses the Phase-1 recursive-descent mindset and lets us keep an
  **insertion-ordered** object so output matches real jq's key order (Go's
  `encoding/json` decodes objects into an order-randomising `map[string]any`).
  Trade-off: `encoding/json` would be fewer lines and battle-tested, but it would
  hide the very concept this challenge is meant to teach, and we'd have to bolt
  ordering back on anyway. For a *production* tool, prefer `encoding/json` +
  `json.Decoder` for streaming.
- **Eager slices, not lazy generators.** Real jq streams values lazily; we
  collect each stage into a `[]any` for readability. Fine for CLI-sized inputs;
  a streaming version would use Go channels or a callback.
- **`values` semantics.** Real jq's `values` builtin *filters out nulls from a
  stream* (`select(. != null)`). The challenge brief lists `values` alongside
  `keys`, so we implement the intuitive "give me the object's values / array's
  elements" meaning instead. This divergence is called out in `eval.go` too.

## 📖 Further Reading

- [codingchallenges.fyi — Build your own jq](https://codingchallenges.fyi/challenges/challenge-jq/)
- [Official jq manual](https://jqlang.github.io/jq/manual/) — the full filter language
- [`docs/go-quickstart.md`](../../docs/go-quickstart.md) — the project's Go-for-Python-devs primer
- [Phase-1 JSON Parser](../../phase-01-foundations/json-parser/) — the `lex → parse → evaluate` skeleton this builds on
- *Crafting Interpreters* (Robert Nystrom) — the canonical tree-walking-interpreter book
