# Shell (`gosh`)

> **Phase:** 3 — Advanced CLI & Orchestration · **Challenge:** 22 (🏁 Phase Capstone)
> **Difficulty:** 🟠 · **Recommended Language:** 🟦 Go · **Effort Estimate:** L

**Status:** ✅ Done

> 🐍→🐹 **New to Go?** Read [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
> first — it teaches Go *by mapping it to Python*, using this repo's own code as
> the running example. Every Go idiom in the source below is also commented
> inline, so keep the `.go` files open side-by-side with this README.

---

## 🎯 What We're Building

A **shell** — the program that runs every other program. When you type

```
$ sort < names.txt | uniq -c | sort -rn > report.txt
```

something has to read that line, figure out it's *three* programs joined by
pipes with two file redirections, launch all three at once, stream bytes between
them through the kernel, and report the final exit status. That "something" is a
shell. It is the **orchestrator** — the conductor of every Unix tool you built
in Phase 2.

`gosh` is a small but genuinely working interactive shell. It supports:

| Feature | Example |
|---|---|
| Prompt + REPL | `gosh dir$ ` reading lines from stdin |
| Quoting & escapes | `echo "hello world"`, `echo 'a $b'`, `echo a\ b` |
| Pipelines | `echo hi \| cat \| wc -l` |
| Redirections | `> out`, `>> log`, `< in`, `2> err` |
| Sequencing | `cmd1 ; cmd2` |
| Logical operators | `make && ./run \|\| echo failed` |
| Builtins | `cd`, `pwd`, `exit`, `echo`, `export`, `type` |
| Variables | `export X=1`, `Y=hi`, `$X`, `${Y}`, `$?`, `$$` |
| Signals | `Ctrl-C` kills the running child, not your shell |
| Scripts | `gosh script.sh` and `gosh -c "echo hi"` |

```bash
go run .                       # interactive REPL
go run . -c "echo hi | wc -c"  # one-shot command string
go run . myscript.sh           # run a script file
```

---

## 📚 Core Concepts

### The shell's job, in one sentence

**A shell is a loop that reads a line, turns it into processes, and waits for
them.** Everything else is detail. The four big ideas:

1. **Tokenizing & parsing** — text → a structured *pipeline tree*.
2. **fork/exec** — how one program launches another (and why it's *two* steps).
3. **Pipes & redirection** — rewiring a child's file descriptors *before* it runs.
4. **Builtins vs externals** — why some commands (like `cd`) can't be child
   processes.

### fork/exec: the two-step that powers Unix

In C, launching a program is `fork()` (clone the current process) then `exec()`
(replace the clone's program image with the new one). The magic window is the
gap *between* those two calls: the child is still running the shell's code, so
the shell can rearrange the child's file descriptors (stdin/stdout/stderr)
**before** the new program takes over. That's how redirection and pipes work.

Go hides `fork`/`exec` behind [`os/exec`](https://pkg.go.dev/os/exec). Setting
`cmd.Stdin`, `cmd.Stdout`, `cmd.Stderr` and calling `cmd.Start()` *is* the
"set up the fds, then exec" dance. `Start()` launches and returns immediately
(like `fork`); `Wait()` blocks for the child to finish.

> 🐍→🐹 Python's `subprocess.Popen(..., stdout=PIPE)` is the same idea.
> `Start()` ≈ `Popen()`, `Wait()` ≈ `.wait()`. Go just makes the fd wiring
> explicit, which is perfect for *learning* what a shell really does.

### Why `cd` **must** be a builtin

This is the single most important systems lesson in the whole challenge.

The current working directory is **per-process state**. If `cd` were an external
program, the shell would `fork`/`exec` a child, the *child* would change *its
own* directory, and then the child would exit — leaving the parent shell exactly
where it started. Nothing would appear to happen.

So `cd` has to run **inside the shell process itself**. The same is true of
anything that mutates shell state: `exit` (must stop *our* loop), `export`
(must change *our* environment so future children inherit it), and variable
assignment (`X=1`). These are the **builtins**. Everything else is fair game for
fork/exec.

---

## 🏗️ Architecture & Design

Three stages, mirroring how any language toolchain works (lexer → parser →
evaluator), one Go file per stage:

```
 raw line ─► [ lexer.go ] ─► tokens ─► [ parser.go ] ─► AST ─► [ executor.go ] ─► processes
              tokenize                   parse                   execList
```

| File | Responsibility |
|---|---|
| `internal/shell/lexer.go` | Tokenizer: split into words honouring quotes/escapes; emit operators |
| `internal/shell/parser.go` | Recursive-descent parser → pipeline AST |
| `internal/shell/expand.go` | `$VAR` / `${VAR}` / `$?` / `$$` expansion |
| `internal/shell/builtins.go` | In-process commands (`cd`, `pwd`, …) + the dispatch table |
| `internal/shell/executor.go` | fork/exec, pipe & redirect wiring, `&&`/`\|\|`/`;` logic |
| `internal/shell/repl.go` | Interactive prompt loop, signals, script runner |
| `internal/shell/shell.go` | Shared `Shell` state + the `tokenize→parse→execute` glue |
| `main.go` | Tiny entry point: pick mode (interactive / `-c` / file) |

### The AST (what `parser.go` builds)

The grammar encodes operator precedence by nesting (loosest on the outside):

```
List      ::= AndOr (';' AndOr)*               ; sequences independent commands
AndOr     ::= Pipeline (('&&' | '||') Pipeline)*
Pipeline  ::= Command ('|' Command)*           | streams stdout → next stdin
Command   ::= (Word | Redirection)+            a program + args + redirects
```

So `a && b | c ; d` parses as:

```
List
├── AndOr:  a  &&  Pipeline(b | c)
└── AndOr:  d
```

---

## 🔧 What `cmd1 | cmd2 > file` *actually* does under the hood

This is the question the whole capstone exists to answer. Here is the fd-level
picture the executor builds for `cmd1 | cmd2 > file`:

```
        ┌──────────┐                     ┌──────────┐
 stdin  │          │   the pipe          │          │
 ──────▶│  cmd1    │  ┌──────────────┐   │  cmd2    │
        │          │  │              │   │          │
        │  fd1 ────┼──┤ w0  ░░░  r0  ├───┼─▶ fd0    │
        │  (stdout)│  │  (kernel buf)│   │  (stdin) │
        └──────────┘  └──────────────┘   │  fd1 ────┼──▶  file
                                         │  (stdout)│      open(O_CREAT|O_TRUNC)
                                         └──────────┘
```

Step by step, exactly as `executor.go` does it:

1. **Create the pipe.** `os.Pipe()` returns a `(reader, writer)` pair of
   file descriptors connected by an in-kernel buffer. Bytes written to `w0`
   come out of `r0`.
2. **Wire stage 1.** `cmd1.Stdout = w0`. Its stdin stays the terminal.
3. **Wire stage 2.** `cmd2.Stdin = r0`. Its stdout is redirected: we
   `os.Create("file")` and set `cmd2.Stdout = file`.
4. **Start *both* children.** They run **concurrently** — a pipeline is parallel,
   not sequential. `cmd1` can be blocked writing while `cmd2` is reading.
5. **The crucial cleanup.** After starting the children, the **parent shell must
   close its own copies** of `w0` and `r0`. Each `exec` gave the child a *dup* of
   the fd, so the child holds the only copies it needs. If the parent keeps `w0`
   open, then when `cmd1` exits, the pipe still has a writer (us!) — so `cmd2`'s
   read **never sees EOF** and the pipeline **hangs forever.** This off-by-one fd
   bug is the classic shell-writing trap; see the `parentCloses` handling and the
   "ownership rule" comment in `execMulti`.
6. **Wait.** The shell `Wait()`s on every stage. A pipeline's exit status is the
   status of its **last** stage.

> 🔑 **The headline takeaway:** a pipe is just two file descriptors plus a kernel
> buffer, and a redirect is just `open()`ing a file and assigning it to fd 0, 1,
> or 2 *before* exec. Once you've wired the fds, the kernel does all the
> streaming for you.

---

## 🔨 Step-by-Step Implementation

### 1. Tokenize (`lexer.go`)

Scan the line byte-by-byte. Outside quotes, whitespace splits words and the
operator characters (`| ; & > <`) emit operator tokens. Quoting rules:

- **single quotes** `'…'` — everything literal (no expansion, no escapes);
- **double quotes** `"…"` — spaces literal, but `$` still expands; backslash only
  escapes `" \ $ \``;
- **backslash** `\x` — the next character is literal.

Each word is stored as a list of `wordPart{text, expand}` so we remember *which
pieces were quoted* — essential so `'$HOME'` stays literal while `"$HOME"`
expands. Glued forms like `2>file` and `a"b"c` are handled.

### 2. Parse (`parser.go`)

A textbook recursive-descent parser, one method per grammar rule, producing the
AST above. Redirections attach to the `Command` they follow.

### 3. Expand (`expand.go`)

Just before execution, expandable word-parts get `$NAME`, `${NAME}`, `$?` (last
exit status) and `$$` (pid) substituted. Single-quoted parts are skipped.

### 4. Execute (`executor.go`)

- `execList` → `execAndOr` (the `&&`/`||` short-circuit logic) → `execPipeline`.
- **Single command** (`execSimple`): if it's a builtin, run it *in-process*
  (so `cd` works); otherwise `exec.Command(...).Run()`. Apply redirections by
  `open()`ing files and assigning the streams.
- **Multi-stage** (`execMulti`): build `n-1` pipes, wire each stage, start them
  all, **close the parent's pipe fds**, then `Wait`. Builtins can even appear as
  pipeline stages (they run in a goroutine writing to the pipe).

### 5. REPL & signals (`repl.go`)

The prompt loop reads lines and feeds them to `RunLine`. A `SIGINT` (Ctrl-C)
handler is installed that **swallows** the signal at the shell level: because the
shell and its foreground child share a process group, both receive Ctrl-C — the
child dies (default action) and the shell survives and reprints the prompt.
That's exactly the behaviour you feel in bash.

---

## 🧪 Testing Strategy

```bash
go test ./...   # unit + integration tests
go vet ./...    # static checks
```

The code is split so it's testable **without a terminal** — the `Shell` reads
from an `io.Reader` and writes to `io.Writer`s, so tests wire them to
`bytes.Buffer`.

- **Tokenizer** (`lexer_test.go`): word-splitting, single/double quotes, escapes,
  empty `""`, unterminated-quote errors, all operators, glued `2>file`.
- **Parser** (`parser_test.go`): multi-stage pipelines, the three redirection
  kinds + append, `;` sequencing, `&&`/`||` chains, syntax errors.
- **Expansion** (`builtins_test.go`): `$VAR`, `${VAR}`, `$?`, missing vars,
  single-quote-is-literal.
- **Builtins** (`builtins_test.go`): `echo`/`echo -n`, `cd`+`pwd`, `export`
  reaching the child env, `type`, assignment staying shell-local, `$?` tracking.
- **Integration / real execution** (`executor_test.go`): runs harmless real
  programs — `true`/`false` exit codes, command-not-found → 127, a 2-stage
  `printf | cat`, a 3-stage `printf | cat | wc -l`, redirect-to-tempfile then
  read it back, `>>` append, `2>` stderr capture, `cmd | sort > file`, `;`,
  `&&`/`||` short-circuit, a builtin inside a pipeline, and a multi-line script.

### Try it by hand

```text
gosh dir$ echo one two | cat | wc -w
       2
gosh dir$ echo hello > /tmp/x && cat /tmp/x
hello
gosh dir$ cd / ; pwd
/
gosh dir$ false ; echo $?
1
gosh dir$ type cd ; type ls
cd is a shell builtin
ls is /bin/ls
```

---

## 💡 Key Takeaways

- A shell is fundamentally a **read → parse → fork/exec → wait** loop.
- **`fork`/`exec` is two steps on purpose**: the gap lets you rewire fds before
  the new program runs. That gap *is* redirection and piping.
- **A pipe = two fds + a kernel buffer.** A redirect = `open()` a file and assign
  it to fd 0/1/2 before exec. Nothing more mysterious than that.
- **`cd` (and `exit`, `export`, assignment) must be builtins** because they
  change the shell's *own* process state, which a child can't do for it.
- The **#1 pipeline bug is forgetting to close the parent's pipe fds** — leave a
  writer open and the reader never gets EOF, so it hangs.
- A pipeline runs its stages **concurrently**, and its exit status is the **last**
  stage's.

---

## 📖 Further Reading

- Coding Challenges — [Build Your Own Shell](https://codingchallenges.fyi/challenges/challenge-shell/)
- Go [`os/exec`](https://pkg.go.dev/os/exec) and [`os.Pipe`](https://pkg.go.dev/os#Pipe)
- *The Linux Programming Interface*, Michael Kerrisk — chapters on processes,
  `fork`/`exec`, pipes, and file descriptors
- POSIX Shell Command Language — [grammar reference](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html)
- This repo's [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
