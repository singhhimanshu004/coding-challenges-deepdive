package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// These small helpers let TestStdinFallback drive the full run() path by
// temporarily swapping the process's stdin and stdout. They're kept separate
// so the main test file reads as a list of behavioral cases.

func osPipe() (*os.File, *os.File, error) {
	return os.Pipe()
}

// swapStdin points os.Stdin at r and returns the old value for restoration.
func swapStdin(r *os.File) *os.File {
	old := os.Stdin
	os.Stdin = r
	return old
}

func restoreStdin(old *os.File) {
	os.Stdin = old
}

// captureStdout redirects os.Stdout to an in-memory pipe and returns a closure
// that restores stdout and yields everything written in the meantime.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	return func() string {
		w.Close()
		os.Stdout = old
		return <-done
	}
}
