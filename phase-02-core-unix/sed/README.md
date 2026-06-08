# sed

> **Phase:** 2 — Core Unix: Text Processing  
> **Difficulty:** 🔵  
> **Recommended Language:** 🟦 Go  
> **Effort Estimate:** M

**Status:** ✅ Done

> ��→🐹 **New to Go?** Read [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
> first. It teaches Go *by mapping it to Python*, using this repo's own code as
> the running example. Every Go-specific idiom below is also commented inline in
> the source files, so keep them open side by side.

---

## 🎯 What We're Building

`sed` — the **s**tream **ed**itor — reads text line by line, applies an edit
*script* to each line, and writes the result out. It is the tool you reach for
when you want to transform a file or a pipe non-interactively: rename things,
delete lines, print just the interesting parts.

The single most important idea: **sed is a tiny programming language with a
fixed execution loop.** You don't call sed with options that each do one thing;
you hand it a *script* — a little program in its own mini-language — and sed
*interprets* that script once per input line. Our job is to build that
interpreter.

```bash
sed 's/foo/bar/g' file.txt        # replace every foo with bar
sed -n '2,4p'     file.txt        # print only lines 2–4
printf 'a\nb\nc\n' | sed '$d'     # drop the last line
sed -i 's/old/new/g' notes.txt    # edit the file in place
```

This implementation supports a teaching-sized core:

| Feature | Syntax | Meaning |
|---|---|---|
| **Substitute** | `s/regex/replacement/[g][i][p]` | Replace matches; `\1`..`\9` backrefs, `&` = whole match |
| **Print** | `p` | Print the current line |
| **Delete** | `d` | Drop the current line |
| **Line address** | `3cmd` | Run `cmd` only on line 3 |
| **Last-line address** | `$cmd` | Run `cmd` only on the final line |
| **Regex address** | `/re/cmd` | Run `cmd` on lines matching `re` |
| **Range** | `addr1,addr2 cmd` | Run `cmd` from `addr1` through `addr2` (inclusive) |
| **`-n`** | flag | Suppress automatic printing |
| **`-i`** | flag | Edit files in place |

> **Regex dialect note.** Go's standard library uses **RE2** syntax (the
> ERE-style family), so groups are written `(\w+)` — *not* the BRE `\(\w+\)` that
> GNU sed defaults to. Backreferences in the *replacement* still use sed's `\1`
> form; we translate between the two dialects for you (see below).

## 📚 Core Concepts

### 1. sed is an interpreter, not a flag bag
Every interpreter has two halves: a **parser** (text → instructions) and an
**executor** (run the instructions over data). sed is exactly this. The script
`s/a/b/; 2d` is parsed *once* into two commands, and then those commands are run
against *every* line. Recognising this split is the whole challenge — it is the
same parser/executor shape you'll meet again in the Phase 2 capstone JSON parser
and every later interpreter.

### 2. The pattern space and the execution cycle
sed keeps a one-line scratch buffer called the **pattern space**. The core loop is:

```
for each input line:
    load the line into the pattern space
    for each command in the script:
        if the command's address selects this line:
            run the command (it may rewrite the pattern space)
    unless -n: auto-print the pattern space
```

That "unless -n: auto-print" step is the detail that surprises Python
developers: by default sed **echoes every line**, and commands like `p` add
*extra* output on top. `-n` switches off the automatic echo so you can print
exactly what you choose — which is why `sed -n '2,4p'` is the idiom for "show me
lines 2 to 4."

### 3. Addressing: which lines does a command touch?
A bare command runs on *every* line. Prefix it with an **address** to restrict it:

- a **line number** (`3`) — one specific line,
- the **`$`** marker — the last line (a property of the whole stream, not the line),
- a **`/regex/`** — every line the pattern matches.

Two addresses make a **range**: `addr1,addr2` turns on when `addr1` matches and
stays on until `addr2` matches on a *later* line. That "stays on across lines" is
a little **state machine** living inside each command — the one piece of
cross-line memory in an otherwise line-local model.

### 4. Substitution with backreferences
`s/regex/replacement/` is sed's workhorse. Two subtleties:

- **First vs. global.** Without the `g` flag, only the *first* match on each line
  is replaced. The standard library's `ReplaceAllString` always replaces *all*
  matches, so we walk the matches by hand to honour first-only semantics.
- **Backreferences.** sed writes capture groups as `\1`..`\9` and the whole match
  as `&`. Go's replacement syntax instead uses `${1}` and `$0`. We translate the
  sed dialect into Go's at parse time, so `s/(\w+) (\w+)/\2 \1/` (swap two words)
  just works.

## 🏗️ Architecture & Design

Two layers, mirroring the other Go challenges in this repo (a thin `main.go`
command wrapping a testable `internal/` package):

```
sed/
├── main.go                       # CLI: flag parsing (-n, -i), file/stdin wiring, exit codes
├── internal/sed/
│   ├── command.go                # data model: address + command; applies() and substitute()
│   ├── parser.go                 # Parse: script text → []*command  (the front end)
│   ├── executor.go               # Run: the line-by-line execution loop (the back end)
│   └── sed_test.go               # unit tests for the parser/executor internals
├── main_test.go                  # end-to-end tests driving run() (incl. -i, stdin, files)
├── go.mod
└── .gitignore
```

The data flow is a clean pipeline — the interpreter shape made literal:

```
script text ──Parse()──▶ []*command ──Run(lines, out, opts)──▶ edited text
              (front end)   (the program)     (back end / executor)
```

- **`Parse`** is a small hand-written recursive-descent parser. It reads the
  script with a single cursor and produces a `[]*command`. Keeping it
  hand-rolled (no parser library) means the whole mini-language is visible in
  one file.
- **`command`** is the parsed instruction: optional address(es), the verb
  (`s`/`p`/`d`), and — for `s` — the compiled regex, the translated replacement
  template, and the flags. One struct + a `switch` beats one type per verb when
  there are only three verbs.
- **`Run`** is the executor: it owns the read → execute → auto-print cycle and
  the pattern space. It resets range state at the top so the same compiled
  command list can be reused per file (needed for `-i`).

Why split parsing from execution? Because a bad script is a **usage error**
(exit `2`) we want to catch *before* touching any data, while an I/O failure
mid-stream is a **domain error** (exit `1`). The split makes that mapping fall
out naturally — matching the repo's convention (`0` success · `1` domain failure
· `2` usage error).

## 🔨 Step-by-Step Implementation

1. **Model the data** (`command.go`). Define `address` (a kind + a line number or
   compiled regex) and `command` (addresses + verb + substitution fields). Two
   behaviours live here: `applies` (does this command fire on this line, advancing
   range state) and `substitute` (perform an `s///`).
2. **Parse the script** (`parser.go`). Walk the script with a cursor: read an
   optional address, an optional `,address` for ranges, then the verb. For `s`,
   read the delimiter (any char after `s`), the regex, the replacement, and the
   `g`/`i`/`p` flags. Compile the regex (prefixing `(?i)` for `i`) and translate
   the replacement dialect.
3. **Translate the replacement** (`convertReplacement`). Rewrite `\N` → `${N}`,
   `&` → `${0}`, escape literal `$` as `$$`, and resolve `\n`/`\t`/`\&`/`\\`. This
   is the one spot where sed's and Go's replacement dialects meet.
4. **Run the loop** (`executor.go`). For each line: load the pattern space, run
   each applicable command, handle `d` (skip the rest + suppress auto-print), and
   auto-print unless `-n`.
5. **Wire the CLI** (`main.go`). Parse `-n`/`-i` (individually or clustered like
   `-ni`), take the first positional as the script and the rest as files, then
   route to stdin→stdout, files→stdout, or in-place rewriting.

### Go idioms you'll meet here (with the Python translation)

- **`iota` enums:** `addrLine`/`addrLast`/`addrRegex` are auto-numbered constants
  of a named int type — Go's idiom for a typed enum, since there's no `enum`
  keyword. (`command.go`)
- **Zero values:** an `addrLine` address leaves its `re *regexp.Regexp` field as
  `nil` for free — Go initialises every field, so there's no constructor
  boilerplate. (`command.go`)
- **Methods on structs with state:** `command.applies` mutates `c.active` to carry
  range state *between* lines — a method that remembers, like an object with one
  field. (`command.go`)
- **Multiple return values:** `substitute` returns `(string, bool)` — the new line
  *and* whether anything changed — with no tuple object, like Python unpacking but
  native. (`command.go`)
- **`defer`:** `defer w.Flush()` schedules the buffer flush to run on return —
  Go's `try/finally`. (`executor.go`)
- **Interfaces for I/O:** `Run(..., out io.Writer, ...)` accepts *anything*
  writable, so production passes `os.Stdout`, in-place editing passes a
  `strings.Builder`, and tests pass a `bytes.Buffer`. (`executor.go`, `main.go`)
- **The `run(args, in, out, err) int` seam:** `main` is one line; the real logic is
  a testable function returning an exit code — the pattern every Go tool in this
  repo uses. (`main.go`)

## 🧪 Testing Strategy

Run everything from this directory:

```bash
go vet ./...     # static checks — must be clean
go test ./...    # unit + end-to-end tests — must pass
```

`main_test.go` drives the real `run()` seam end to end and covers:

- `s///` first-only vs. `g` (global) vs. `i` (case-insensitive)
- backreferences (`\2 \1` word-swap) and the whole-match `&`
- an alternate delimiter (`s|…|…|`)
- `p` (prints twice without `-n`) and `-n … p` (print only selected lines)
- `d` with numeric, `$`, and `/regex/` addresses
- numeric ranges (`2,4d`) and **regex ranges** (`/BEGIN/,/END/p`)
- addressed substitution (`2s/…/…/`) and multi-command scripts (`s/…/; 2d`)
- reading from **stdin**, from a **file argument**, and **`-i` in-place** edit on
  a temp file (asserting the file is rewritten and stdout stays empty)
- usage errors (missing script, bad syntax, unknown command) → exit code `2`

`internal/sed/sed_test.go` unit-tests the tricky internals directly:
`convertReplacement` dialect translation, parser error cases, first-vs-global
substitution, the single-line range state machine, and line splitting.

### Differential testing against the real `sed`
The implementation was also checked **against the system `sed`** byte-for-byte
across a batch of scripts by piping the same input to both and diffing — all
matched. You can reproduce the idea quickly:

```bash
go build -o sed .
IN=$'apple\nbanana\ncherry\ndate\n'
for s in 's/a/A/g' '2,3d' '$d' '/an/d' '2p'; do
  a=$(printf '%s' "$IN" | ./sed "$s")
  b=$(printf '%s' "$IN" | /usr/bin/sed "$s")
  [ "$a" = "$b" ] && echo "OK   $s" || echo "DIFF $s"
done
rm -f sed
```

## 💡 Key Takeaways

- **A tool can be a language.** sed's power comes from being a tiny interpreter:
  parse a script once, run it over a stream. Internalising the parser/executor
  split is the transferable skill — it reappears in every interpreter you build.
- **The pattern space + auto-print loop** is the whole execution model. Once you
  see "read line → run commands → auto-print (unless `-n`)," every sed one-liner
  becomes readable.
- **Addressing is a filter; ranges are a state machine.** Single addresses are
  line-local predicates; ranges carry one bit of cross-line state. Knowing how
  much state an operation needs is a core stream-processing instinct.
- **Mind the dialects.** sed and Go disagree on regex syntax (BRE vs. RE2) and on
  replacement backreferences (`\1` vs. `${1}`). Translating at the boundary keeps
  the rest of the code clean.
- **Differential testing is gold** for clones: diffing against the real `sed`
  catches edge cases no hand-written assertion would think to check.

## 📖 Further Reading

- Coding Challenges — [Build Your Own `sed`](https://codingchallenges.fyi/challenges/challenge-sed/)
- GNU sed manual — [Execution cycle & commands](https://www.gnu.org/software/sed/manual/sed.html)
- POSIX spec — [`sed`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/sed.html)
- Go package — [`regexp` syntax (RE2)](https://pkg.go.dev/regexp/syntax)
- Repo primer — [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
