# crontab

> **Phase:** 3 — Advanced CLI & Orchestration
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`struct`, bitsets, the `time` package,
> error returns, injected I/O streams) back to the Python you already know.
> This README assumes you've skimmed it.

---

## 🎯 What We're Building

A tool that **understands cron expressions**. Cron is the scheduler that has run
the world's nightly backups and report jobs for forty years, and its scheduling
language is a tiny five-character grammar that looks innocent and hides real
subtlety.

Our `crontab` does three things:

1. **Parse & validate** a cron expression like `*/15 9-17 * * 1-5`.
2. **Explain** it in plain English.
3. **Compute the next N run times** from any reference instant.

```text
$ crontab -n 5 "*/15 9-17 * * 1-5"
Next 5 run(s) after Tue, 09 Jun 2026 02:27:16 IST:
  Tue, 09 Jun 2026 09:00:00 IST
  Tue, 09 Jun 2026 09:15:00 IST
  Tue, 09 Jun 2026 09:30:00 IST
  Tue, 09 Jun 2026 09:45:00 IST
  Tue, 09 Jun 2026 10:00:00 IST
```

We deliberately **do not** daemonize and run jobs — the interesting computer
science here is *expression parsing* and *schedule math*, not `fork`/`exec`.
(There is an optional `-run` mode that sleeps until the next tick, as a bonus.)

---

## 📚 Core Concepts

### The five-field grammar

A standard cron line is five space-separated fields:

```text
┌──────────── minute        (0-59)
│ ┌────────── hour          (0-23)
│ │ ┌──────── day-of-month  (1-31)
│ │ │ ┌────── month         (1-12 or JAN-DEC)
│ │ │ │ ┌──── day-of-week   (0-6  or SUN-SAT; 0 and 7 both mean Sunday)
│ │ │ │ │
* * * * *
```

| Field        | Range          | Names            |
| ------------ | -------------- | ---------------- |
| minute       | `0–59`         | —                |
| hour         | `0–23`         | —                |
| day-of-month | `1–31`         | —                |
| month        | `1–12`         | `JAN`…`DEC`      |
| day-of-week  | `0–6` (or `7`) | `SUN`…`SAT`      |

> 🐍 `time.Weekday()` in Go uses Sunday = 0, exactly like cron's day-of-week
> field, so they line up with zero translation. We normalize the alias `7`
> (also Sunday) down to `0` at parse time.

### What each field can contain

Every field is a comma-separated **list** of terms, and each term is one of:

| Syntax     | Meaning                                  | Example expands to            |
| ---------- | ---------------------------------------- | ----------------------------- |
| `*`        | every value in the field's range         | minute `*` → 0,1,2,…,59       |
| `a`        | a single value                           | `5` → 5                       |
| `a-b`      | an inclusive range                       | `9-17` → 9,10,…,17            |
| `*/s`      | every `s`-th value across the whole range| `*/15` → 0,15,30,45           |
| `a-b/s`    | every `s`-th value within a range        | `10-30/5` → 10,15,20,25,30    |
| `a/s`      | from `a` to the max, every `s`           | `5/20` (min) → 5,25,45        |
| `JAN`,`MON`| named month / weekday (case-insensitive) | `MON-FRI` → 1,2,3,4,5         |

#### How `*/step` and ranges expand

A step always rides on top of a **range**. `*/15` is really "the full range
`0-59`, take every 15th value." `10-30/5` is "the range `10-30`, every 5th
value." The algorithm is identical in both cases — pick the low/high bounds,
then loop `for v := lo; v <= hi; v += step`. `*` is just shorthand for "the
field's whole legal range," so once you resolve `*` to `min-max`, steps need no
special case.

### Fields as bitsets — the key idea

Each field is parsed into a **bitset**: a single `uint64` where bit `v` is set
when value `v` is allowed. (The biggest value we store is 59, which fits.)

```go
type field struct {
    bits   uint64 // bit v set ⇒ value v matches
    isStar bool   // was the raw token a bare "*"?
}
func (f field) has(v int) bool { return f.bits&(1<<uint(v)) != 0 }
```

> 🐍 A `field` is morally a `set[int]` of allowed values, packed into one
> integer. `f.has(5)` is `5 in allowed_minutes`. Once every field is a set,
> "does time T match?" is just five membership tests AND'd together.

### Macros

Cron ships convenience shorthands, which we expand to a five-field form before
parsing:

| Macro                  | Equivalent  |
| ---------------------- | ----------- |
| `@hourly`              | `0 * * * *` |
| `@daily` / `@midnight` | `0 0 * * *` |
| `@weekly`              | `0 0 * * 0` |
| `@monthly`             | `0 0 1 * *` |
| `@yearly` / `@annually`| `0 0 1 1 *` |

### ⚠️ The day-of-month vs. day-of-week gotcha (the OR rule)

This is the single most surprising thing about cron, and the bug most homemade
parsers get wrong:

> **When BOTH the day-of-month and day-of-week fields are restricted (neither is
> `*`), a day matches if _either_ one matches. It is an OR, not an AND.**

So `0 0 13 * 5` fires at midnight on **the 13th of every month _and_ on every
Friday** — not only on Friday the 13th. You can watch it happen:

```text
$ crontab -n 2 "0 0 13 * 5"
  Fri, 12 Jun 2026 00:00:00 IST   ← a Friday (the day-of-week branch)
  Sat, 13 Jun 2026 00:00:00 IST   ← the 13th  (the day-of-month branch)
```

When one of the two fields **is** `*`, that field adds no constraint and the
other decides alone — which collapses back to ordinary AND behaviour. That's why
we store `domStar` / `dowStar` booleans alongside the bitsets: they're the only
way to tell "the user wrote `*`" apart from "every value happened to be listed."

---

## 🏗️ Architecture & Design

A clean three-stage split, one concern per file:

```text
parser.go    cron string → 5 bitsets   (Parse, parseField, parseTerm, parseValue)
schedule.go  bitsets → next-run math    (Matches, Next, NextN, dayMatches)
explain.go   bitsets → human English    (Explain, describeField)
cli.go       argv → output streams      (run, parseArgs)
main.go      os.Args + os.Exit only
```

`main()` is intentionally tiny: it calls `run(args, stdout, stderr)` and exits.
`run` takes its output streams as `io.Writer` parameters, so tests drive it with
`bytes.Buffer`s — no subprocess, no temp files. (This is the same shape used by
every Go tool in this repo; see the `wc`/`cut`/`tr` write-ups.)

---

## 🔨 Step-by-Step Implementation

1. **`parseValue`** — the atom: a number or a name (`JAN`, `MON`), bounds-checked.
2. **`parseTerm`** — one comma-free term. Peel off an optional `/step`, resolve
   the range part (`*`, `a`, or `a-b`), then stamp `lo..hi` step values into the
   bitset.
3. **`parseField`** — split on commas, OR each term's bits together; remember if
   the raw token was `*`.
4. **`Parse`** — expand macros, split into exactly five fields, parse each with
   the right bounds/name-table, record `domStar` / `dowStar`.
5. **`dayMatches`** — the OR rule, branching on the two star flags.
6. **`Next`** — the schedule math (below).
7. **`Explain` / CLI** — presentation.

### The next-run-time algorithm

Naively you could step minute-by-minute and test `Matches`, but a yearly job
would loop half a million times. Instead we **jump in the biggest unit that's
wrong**, from coarsest to finest:

```text
t := reference truncated to the minute, + 1 minute   // "strictly after"
loop:
  if month  doesn't match → jump to 00:00 on the 1st of next month
  else if day doesn't match → jump to 00:00 tomorrow
  else if hour doesn't match → jump to the start of the next hour
  else if minute doesn't match → add one minute
  else → every field matches; return t
```

Each "jump" uses `time.Date(...)`, which makes calendar arithmetic correct for
free: month `12 + 1` rolls into January of the next year, and `AddDate(0,0,1)`
handles month/year lengths and leap years. A 5-year safety bound means an
impossible expression like `0 0 30 2 *` (Feb 30th) terminates and reports "no
upcoming run" instead of spinning forever.

`NextN` simply feeds each result back in as the new reference to walk the
schedule forward.

---

## 🧪 Testing Strategy

`crontab_test.go` covers every layer:

- **Field syntax** — `*`, single values, lists, ranges, `*/step`, `a-b/step`,
  `a/step`, named months, named weekday ranges, the `7`→Sunday alias.
- **Macros** — each macro equals its hand-written five-field form.
- **Invalid expressions** — out-of-range values, reversed ranges, zero steps,
  bad names, wrong field counts, empty list terms, unknown macros.
- **Next-run math** against **fixed UTC reference times**: simple stepping, the
  strictly-after boundary, business-hours roll to Monday, **month** rollover,
  **year** rollover, the **dom/dow OR rule** (both branches), a dom-only field
  (no OR), an **impossible** Feb-30th expression, and `NextN` ordering.
- **CLI** — `run()` exit codes and output via in-memory buffers.

```bash
go test ./...   # all green
go vet ./...    # clean
```

> All reference times in the tests are explicit `time.Date(..., time.UTC)`
> values so the suite is deterministic regardless of the machine's time zone.

---

## 💡 Key Takeaways

- **Bitsets turn parsing into membership tests.** Once each field is a `uint64`
  of allowed values, matching a time is five `&` checks — fast and trivial.
- **The OR rule is cron's defining quirk.** Day-of-month and day-of-week combine
  with OR when both are set; you must track whether each was literally `*` to
  get it right.
- **Jump, don't crawl.** Computing the next run by skipping the largest wrong
  unit makes even `@yearly` resolve in a handful of iterations.
- **`time.Date` is your calendar engine.** Lean on it for rollovers and leap
  years instead of reinventing month-length math.
- **Thin `main`, injected streams.** Keeping all logic in `run(...)` with
  `io.Writer` params makes the CLI testable without spawning processes.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the
  idioms (bitsets, `time`, structs, injected I/O) used here, mapped to Python.
- [Coding Challenges — Build Your Own crontab](https://codingchallenges.fyi/challenges/challenge-cron/)
- `man 5 crontab` — the canonical field grammar and the OR-rule wording.
- [Paul Vixie's cron](https://github.com/vixie/cron) — the implementation whose
  behaviour (including the day-OR rule) became the de-facto standard.
