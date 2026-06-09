package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---- tokenize ----

func TestTokenizeWhitespace(t *testing.T) {
	got, err := tokenize(strings.NewReader("a b\tc\nd  e\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTokenizeBlankAndPadding(t *testing.T) {
	got, _ := tokenize(strings.NewReader("   \n\n  hello   world \n\n"), false)
	want := []string{"hello", "world"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTokenizeNulDelimited(t *testing.T) {
	// -0 keeps spaces/newlines inside an item; only NUL splits.
	got, err := tokenize(strings.NewReader("a b\x00c\nd\x00last\x00"), true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a b", "c\nd", "last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

// ---- buildJobs ----

func argvs(jobs []job) [][]string {
	out := make([][]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.argv
	}
	return out
}

func TestBuildBatchDefaultAllInOne(t *testing.T) {
	jobs := buildJobs([]string{"echo"}, []string{"a", "b", "c"}, "", 0)
	want := [][]string{{"echo", "a", "b", "c"}}
	if !reflect.DeepEqual(argvs(jobs), want) {
		t.Fatalf("got %v want %v", argvs(jobs), want)
	}
}

func TestBuildBatchN1(t *testing.T) {
	jobs := buildJobs([]string{"echo"}, []string{"a", "b", "c"}, "", 1)
	want := [][]string{{"echo", "a"}, {"echo", "b"}, {"echo", "c"}}
	if !reflect.DeepEqual(argvs(jobs), want) {
		t.Fatalf("got %v want %v", argvs(jobs), want)
	}
}

func TestBuildBatchN2Remainder(t *testing.T) {
	jobs := buildJobs([]string{"echo", "x"}, []string{"a", "b", "c"}, "", 2)
	want := [][]string{{"echo", "x", "a", "b"}, {"echo", "x", "c"}}
	if !reflect.DeepEqual(argvs(jobs), want) {
		t.Fatalf("got %v want %v", argvs(jobs), want)
	}
}

func TestBuildBatchEmptyRunsOnce(t *testing.T) {
	jobs := buildJobs([]string{"echo"}, nil, "", 0)
	want := [][]string{{"echo"}}
	if !reflect.DeepEqual(argvs(jobs), want) {
		t.Fatalf("got %v want %v", argvs(jobs), want)
	}
}

func TestBuildReplaceOnePerItem(t *testing.T) {
	jobs := buildJobs([]string{"mv", "{}", "backup/{}"}, []string{"a.txt", "b.txt"}, "{}", 0)
	want := [][]string{
		{"mv", "a.txt", "backup/a.txt"},
		{"mv", "b.txt", "backup/b.txt"},
	}
	if !reflect.DeepEqual(argvs(jobs), want) {
		t.Fatalf("got %v want %v", argvs(jobs), want)
	}
}

func TestBuildReplaceEmptyRunsNothing(t *testing.T) {
	jobs := buildJobs([]string{"echo", "{}"}, nil, "{}", 0)
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs, got %v", argvs(jobs))
	}
}

// ---- exit-status propagation ----

func TestAggregateExitPriority(t *testing.T) {
	cases := []struct {
		codes []int
		want  int
	}{
		{[]int{0, 0, 0}, 0},
		{[]int{0, 5, 0}, 123},
		{[]int{0, 126, 5}, 126},
		{[]int{123, 127, 126}, 127},
	}
	for _, c := range cases {
		if got := aggregateExit(c.codes); got != c.want {
			t.Errorf("aggregateExit(%v) = %d, want %d", c.codes, got, c.want)
		}
	}
}

func TestRunJobsPropagatesFailure(t *testing.T) {
	jobs := buildJobs([]string{"cmd"}, []string{"ok", "bad", "ok"}, "", 1)
	fake := func(argv []string) int {
		if argv[1] == "bad" {
			return 1
		}
		return 0
	}
	if got := runJobs(jobs, 1, false, &bytes.Buffer{}, fake); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
}

// ---- -t echo ----

func TestRunJobsEchoesCommands(t *testing.T) {
	jobs := buildJobs([]string{"echo"}, []string{"a", "b"}, "", 1)
	var echoBuf bytes.Buffer
	noop := func(argv []string) int { return 0 }
	runJobs(jobs, 1, true, &echoBuf, noop)
	out := echoBuf.String()
	if !strings.Contains(out, "echo a") || !strings.Contains(out, "echo b") {
		t.Fatalf("echo output missing commands: %q", out)
	}
}

// ---- -P bounded parallelism (deterministic) ----

// TestRunJobsBoundedParallelism deterministically proves two things:
//  1. at most P children run at once (the semaphore upper bound), and
//  2. exactly P run simultaneously when there are >= P jobs (a barrier that
//     only releases once the P-th goroutine arrives — if the pool allowed
//     fewer than P concurrent, the barrier would never open and the test
//     would time out).
func TestRunJobsBoundedParallelism(t *testing.T) {
	const P = 3
	jobs := make([]job, 9)
	for i := range jobs {
		jobs[i] = job{argv: []string{"x"}}
	}

	var active, maxActive int32
	var arrived int32
	release := make(chan struct{})

	fake := func(argv []string) int {
		n := atomic.AddInt32(&active, 1)
		for { // record running maximum
			m := atomic.LoadInt32(&maxActive)
			if n <= m || atomic.CompareAndSwapInt32(&maxActive, m, n) {
				break
			}
		}
		// Barrier: the P-th simultaneous arrival opens the gate for everyone.
		if atomic.AddInt32(&arrived, 1) == int32(P) {
			close(release)
		}
		<-release
		atomic.AddInt32(&active, -1)
		return 0
	}

	done := make(chan int, 1)
	go func() { done <- runJobs(jobs, P, false, &bytes.Buffer{}, fake) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out — pool never reached P concurrent goroutines")
	}

	if got := atomic.LoadInt32(&maxActive); got != P {
		t.Fatalf("max concurrency = %d, want exactly %d", got, P)
	}
}

// ---- end-to-end with a real harmless process ----

func TestRunEndToEndRealEcho(t *testing.T) {
	var out, errBuf bytes.Buffer
	// printf 'a\nb\nc\n' | xargs -n1 echo   (serial to keep buffer writes safe)
	code := run([]string{"-n1", "echo"}, strings.NewReader("a\nb\nc\n"), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errBuf.String())
	}
	lines := strings.Fields(out.String())
	sort.Strings(lines)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %q want %q", lines, want)
	}
}

func TestRunEndToEndExitStatusFromChild(t *testing.T) {
	// `false` always exits 1 → xargs reports 123.
	code := run([]string{"-n1", "false"}, strings.NewReader("a\nb\n"), &bytes.Buffer{}, &bytes.Buffer{})
	if code != 123 {
		t.Fatalf("expected 123 from failing child, got %d", code)
	}
}

func TestRunEndToEndParallelReal(t *testing.T) {
	// Real spawn with -P. Route child stdout through an os.Pipe: because it is
	// a real *os.File, os/exec hands the fd straight to each child, so there is
	// no shared in-process buffer to race on.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	code := run([]string{"-P4", "-n1", "echo"}, strings.NewReader("1 2 3 4 5 6 7 8"), w, w)
	w.Close()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	data, _ := io.ReadAll(r)
	got := strings.Fields(string(data))
	sort.Strings(got)
	want := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

// ---- parseArgs ----

func TestParseArgsGluedAndSeparated(t *testing.T) {
	opts, cmd, err := parseArgs([]string{"-0", "-t", "-n2", "-P", "4", "-I", "{}", "echo", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.nulDelim || !opts.echo || opts.maxItems != 2 || opts.parallel != 4 || opts.replace != "{}" {
		t.Fatalf("bad opts: %+v", opts)
	}
	if !reflect.DeepEqual(cmd, []string{"echo", "hi"}) {
		t.Fatalf("bad command: %v", cmd)
	}
}

func TestParseArgsBadN(t *testing.T) {
	if _, _, err := parseArgs([]string{"-n0", "echo"}); err == nil {
		t.Fatal("expected error for -n0")
	}
}
