# grep

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🆕 **New to Go?** Read the project's [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
> first — it maps slices, structs, `error` returns, interfaces, methods, and
> `defer` onto the Python you already know. Every Go idiom used below is
> cross-referenced there, and the code comments call them out as you meet them.

---

## 🎯 What We're Building

`grep` ("**g**lobally search a **r**egular **e**xpression and **p**rint") is the
search tool of Unix. You give it a **pattern** and some **input**, and it prints
every line that matches:

```console
$ printf 'apple\nbanana\ncherry\n' | grep an
banana

$ grep -rn 'TODO' ./src        # every TODO under ./src, with file:line prefixes
src/app.go:42:// TODO: handle retries
src/db.go:7:// TODO: connection pool
```

Think of it as Python's
`[line for line in lines if re.search(pattern, line)]` — but generalised over
files, standard input, and whole directory trees, with a dozen flags that change
*what* counts as a match and *how much* of the surrounding text you see.

This implementation supports:

| Flag        | Meaning                                                  |
| ----------- | -------------------------------------------------------- |
| _(none)_    | print lines matching the regex                           |
| `-i`        | case-insensitive match                                   |
| `-v`        | **invert** — print lines that do *not* match             |
| `-n`        | prefix each line with its 1-based line number            |
| `-c`        | print only a **count** of matching lines per file        |
| `-w`        | match whole **words** only                               |
| `-r`        | **recurse** into directories                             |
| `-l`        | print only the **names** of files that contain a match   |
| `-A N`      | print `N` lines of context **after** each match          |
| `-B N`      | print `N` lines of context **before** each match         |
| `-C N`      | print `N` lines of context **around** each match         |

Short flags bundle (`-ivn`), context values can be attached (`-A3`) or separate
(`-A 3`), and with no file argument (or `-`) grep reads standard input.

## 📚 Core Concepts

### 1. The pipe-and-filter model

grep is the archetypal Unix **filter**: it reads a stream of lines, keeps the
ones you asked for, and writes them to another stream. It doesn't care whether
its input came from a file, a keyboard, or another program's output — that's the
whole philosophy behind shell pipelines:

```console
$ cat access.log | grep ' 500 ' | grep -v '/health' | wc -l
```

Each stage does one thing and passes the result along. Our implementation keeps
that spirit by taking `io.Reader`/`io.Writer` interfaces (not concrete files)
everywhere, so a real file, `os.Stdin`, or an in-memory test buffer are all
interchangeable. (In Go, accepting an interface instead of a concrete type is
the equivalent of Python's "duck typing" — anything with a `Read` method fits.)

### 2. Regular expressions, and why Go uses RE2

A **regular expression** is a tiny language for describing sets of strings:
`a.c` matches `abc`, `axc`, …; `foo|bar` matches either word; `[0-9]+` matches a
run of digits. grep's job is, for each line, to answer "does this regex match
somewhere in here?"

Go's `regexp` package is built on **RE2**, an engine with a very deliberate
design choice that is worth understanding because it's the heart of this
challenge:

> **RE2 guarantees that matching runs in time linear in the length of the input,
> and it can never "blow up".**

Compare that to the **backtracking** engines used by Perl, PCRE, Java, and
Python's `re`. Those engines explore the regex by trial and error: when a branch
fails, they back up and try another. That's flexible — it's how they support
**backreferences** (`(\w+)\1`) and **lookaround** (`(?=foo)`) — but it has a
dark side called **catastrophic backtracking**. A pattern like `(a+)+$` against
a long string of `a`s followed by a `!` can take *exponential* time, freezing
the program. This is a real, weaponisable denial-of-service bug (a "ReDoS").

| | **RE2 (Go)** | **Backtracking (Python `re`, PCRE, Java)** |
| --- | --- | --- |
| **How it works** | Builds an automaton (NFA/DFA), tracks all possible states at once | Tries one path, backs up on failure, retries |
| **Worst case** | **O(n)** — linear, always | **O(2ⁿ)** — can be exponential (ReDoS) |
| **Backreferences** | ❌ not supported (they make linear time impossible) | ✅ supported |
| **Lookaround** | ❌ not supported | ✅ supported |
| **Best for** | Untrusted input, servers, "must not hang" | Rich patterns on trusted, small inputs |

The tradeoff is a feature for a tool like grep: it will *never* hang on a
pathological pattern, at the cost of dropping two features (backreferences and
lookaround) that line-search rarely needs. When you call `regexp.Compile`, you
are opting into that guarantee. The
[official write-up](https://swtch.com/~rsc/regexp/regexp1.html) by Russ Cox is
the canonical explanation.

### 3. Building flags *into* the pattern

Two flags don't need any special matching code — we just rewrite the pattern
text before compiling it:

- **`-i` (ignore case)** → prepend the inline flag `(?i)`. RE2 reads `(?i)foo`
  as "match `foo` case-insensitively". Portable and engine-native.
- **`-w` (whole word)** → wrap the pattern as `\b(?:PATTERN)\b`. `\b` is a
  *word boundary* (the empty space between a word char and a non-word char).
  The `(?:...)` is a **non-capturing group**: it bundles the user's pattern so
  the boundaries apply to the *whole* thing, even for an alternation like
  `\b(?:foo|bar)\b`. Without the group, `\bfoo|bar\b` would mean "`\bfoo` OR
  `bar\b`" — a classic precedence bug.

`-v` (invert) is the only flag handled at match time: we compute the match, then
flip the boolean.

### 4. Recursive directory walks with `filepath.WalkDir`

`-r` turns grep loose on a directory tree. Go's standard library gives us
`filepath.WalkDir` (Go 1.16+), which visits every entry under a root in sorted
order and calls a function we supply for each one:

```go
filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
    if err != nil { /* report, keep going */ return nil }
    if d.IsDir() { return nil }            // descend automatically
    if !d.Type().IsRegular() { return nil } // skip symlinks/devices/sockets
    // …read and search this file…
    return nil
})
```

The callback's return value steers the walk: `nil` = keep going, `fs.SkipDir` =
prune this subtree, any other error = abort. `WalkDir` is the newer, faster
cousin of `filepath.Walk` — it hands you a lightweight `fs.DirEntry` instead of
doing a full `os.Stat` on every file, which matters on big trees.

### 5. Context lines (`-A` / `-B` / `-C`)

Sometimes the matching line alone isn't enough — you want to see what's *around*
it (the function signature above a match, the stack frame below it). That's
context:

- `-A N` — **A**fter: N lines following each match
- `-B N` — **B**efore: N lines preceding each match
- `-C N` — both (**C** for context); `-C 2` ≡ `-A 2 -B 2`

To look both backwards and forwards around a match, we read each file fully into
a slice of lines first (see the tradeoff note below), then expand every matching
index `i` into the span `[i-B, i+A]`, **merge** overlapping or touching spans so
no line prints twice, and print non-adjacent blocks separated by a literal `--`,
exactly like GNU grep. Matching lines use a `:` separator after the prefix
fields; pure context lines use `-`, so you can always tell which line actually
matched:

```console
$ printf 'one\ntwo match\nthree\n' | grep -n -C1 match
1-one
2:two match
3-three
```

## 🏗️ Architecture & Design

The program is split so each file owns exactly one stage of the pipeline — the
same "one job per piece" discipline grep itself preaches:

```
argv ─▶ main.go ─▶ matcher.go ─▶ walker.go ─▶ output.go ─▶ stdout
        (parse)    (compile)     (gather)     (report)
```

| File           | Responsibility                                                              |
| -------------- | --------------------------------------------------------------------------- |
| `main.go`      | Parse argv (bundled flags, attached values), wire everything, set exit code |
| `matcher.go`   | Compile the pattern into a `Matcher`; decide per-line hits (`-i`,`-v`,`-w`)  |
| `walker.go`    | Turn operands into named `Source`s: files, stdin, or a recursive walk (`-r`) |
| `output.go`    | The reporting engine: counts (`-c`), file lists (`-l`), lines, context, `-n` |
| `grep_test.go` | Table- and tree-based tests for every flag and every exit code              |

The three core types are deliberately small:

- **`Matcher`** — a compiled `*regexp.Regexp` plus the `invert` flag. All failure
  (a bad pattern) happens once in `NewMatcher`; the per-line `Match` can't error,
  keeping the hot loop trivial.
- **`Source`** — a display name plus its lines. Hiding files, stdin, and walked
  files behind one type means the reporting engine never has to care where input
  came from.
- **`Config`** — the fully-parsed request the engine acts on.

### A note on reading whole files into memory

We load each `Source` fully (`[]string` of lines) rather than streaming line by
line. Why? The context flags need to look *backward* (`-B`) and *forward* (`-A`)
around a match, which a forward-only stream can't do without a ring buffer. A
slice makes the logic obvious — at the cost of holding one file in RAM at a time.
Real GNU grep streams with a fixed-size before-context ring buffer; that's a
worthwhile follow-up exercise, noted here so the tradeoff is explicit rather than
accidental.

### Exit codes

grep encodes its result in the **exit status**, which is what lets it drive
shell `if` statements (`if grep -q pat file; then …`):

| Code | Meaning                                          |
| ---- | ------------------------------------------------ |
| `0`  | at least one line matched                        |
| `1`  | no lines matched (a normal, expected outcome!)   |
| `2`  | an error (bad flag, bad pattern, unreadable file)|

Note that "no match" is `1`, not an error — a point that trips up shell scripts
using `set -e`. An operand error forces `2` regardless of whether other files
matched.

## 🔨 Step-by-Step Implementation

1. **Compile the matcher** (`matcher.go`). Rewrite the pattern for `-w`/`-i`,
   then `regexp.Compile`. Surface a compile failure as an `error` → exit 2.
2. **Gather sources** (`walker.go`). No operands → stdin. `-` → stdin. A file →
   read it. A directory under `-r` → `filepath.WalkDir`. A directory without
   `-r` → warn and mark the run errored.
3. **Report** (`output.go`). Branch on mode: `-c` prints a per-file count, `-l`
   prints names of matching files, otherwise print matching lines with optional
   `-n` numbers and `-A/-B/-C` context (merged spans + `--` separators).
4. **Decide the prefix** (`main.go`). Show the `filename:` prefix when there is
   ambiguity — more than one file operand, or any recursive walk.
5. **Set the exit code** (`main.go`). Error → 2; else match → 0, no match → 1.

## 🧪 Testing Strategy

`grep_test.go` drives the real CLI entry point (`cli`) with in-memory streams,
so each test reads like a shell session and asserts on **stdout, stderr, and the
exit code** together. Coverage includes:

- basic match, `-i`, `-v`, `-n`, `-c`, `-w` (including the `foo|bar` alternation
  trap that `-w` must bound correctly)
- `-A`, `-B`, `-C`, and the `--` group separator between non-adjacent blocks
- a **recursive walk over a real temp directory tree** (`t.TempDir()`), checking
  `file:line` prefixes and that non-matching files are excluded
- `-r -l` listing only matching filenames
- multi-file filename prefixing without `-r`
- a directory passed *without* `-r` producing the "Is a directory" warning + exit 2
- **stdin** as the default source
- the **no-match exit code (1)** and the **bad-pattern exit code (2)**

```console
$ go test ./...
ok      grep    0.4s

$ go vet ./...        # clean
```

It was also spot-checked against the system `grep` on the same inputs (context
output, word matching, recursive walks, and exit codes all match).

## 💡 Key Takeaways

- **RE2 vs backtracking is a genuine engineering tradeoff.** Go's linear-time
  guarantee buys immunity to catastrophic backtracking (ReDoS) at the price of
  backreferences and lookaround — the right call for a tool fed untrusted input.
- **Push flags into data where you can.** `-i` and `-w` became pattern rewrites,
  not branches in the matching loop, which keeps the core a single clean call.
- **Model your inputs uniformly.** A `Source` type erased the difference between
  files, stdin, and walked files so the reporting engine stays oblivious to
  origin — the same idea as accepting `io.Reader` instead of `*os.File`.
- **`filepath.WalkDir`** is the modern, allocation-light way to recurse a tree;
  the callback's return value (`nil` / `fs.SkipDir` / error) is the steering wheel.
- **Exit codes are an API.** grep's `0/1/2` convention is what makes it
  composable in shell scripts — "no match" being `1` (not an error) is the
  subtlety to remember.

## 📖 Further Reading

- Challenge spec — [Build Your Own grep](https://codingchallenges.fyi/challenges/challenge-grep/)
- Russ Cox, [Regular Expression Matching Can Be Simple And Fast](https://swtch.com/~rsc/regexp/regexp1.html) — the RE2 design rationale
- Go [`regexp` package](https://pkg.go.dev/regexp) and its [RE2 syntax](https://pkg.go.dev/regexp/syntax)
- Go [`path/filepath.WalkDir`](https://pkg.go.dev/path/filepath#WalkDir)
- [GNU grep manual](https://www.gnu.org/software/grep/manual/grep.html) — the reference behaviour
- Project primer — [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
