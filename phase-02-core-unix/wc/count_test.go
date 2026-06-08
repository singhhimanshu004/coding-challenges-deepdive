package main

import (
	"strings"
	"testing"
)

// TestCount exercises the streaming counter directly: each flag's quantity,
// multibyte handling, and the empty-input edge case all flow through count().
func TestCount(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  counts
	}{
		{
			name:  "empty input",
			input: "",
			want:  counts{lines: 0, words: 0, chars: 0, bytes: 0},
		},
		{
			name:  "single word no newline",
			input: "hello",
			want:  counts{lines: 0, words: 1, chars: 5, bytes: 5},
		},
		{
			name:  "line with trailing newline",
			input: "hello world\n",
			want:  counts{lines: 1, words: 2, chars: 12, bytes: 12},
		},
		{
			name:  "multiple lines and words",
			input: "the quick brown\nfox jumps\n",
			want:  counts{lines: 2, words: 5, chars: 26, bytes: 26},
		},
		{
			name:  "leading and trailing whitespace",
			input: "   spaced   out   \n",
			want:  counts{lines: 1, words: 2, chars: 19, bytes: 19},
		},
		{
			name:  "tabs and mixed whitespace",
			input: "a\tb\tc\n",
			want:  counts{lines: 1, words: 3, chars: 6, bytes: 6},
		},
		{
			// "héllo wörld" — é and ö are 2-byte UTF-8 runes, so bytes > chars.
			name:  "multibyte runes",
			input: "héllo wörld\n",
			want:  counts{lines: 1, words: 2, chars: 12, bytes: 14},
		},
		{
			// Emoji is a single 4-byte rune.
			name:  "emoji rune",
			input: "hi 😀\n",
			want:  counts{lines: 1, words: 2, chars: 5, bytes: 8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := count(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("count() returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("count(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseArgs verifies flag parsing, short-flag bundling and long flags.
func TestParseArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantOpts  options
		wantFiles []string
		wantErr   bool
	}{
		{
			name:      "no args",
			args:      nil,
			wantOpts:  options{},
			wantFiles: nil,
		},
		{
			name:      "single short flags",
			args:      []string{"-l", "-w", "-c", "-m"},
			wantOpts:  options{lines: true, words: true, chars: true, bytes: true},
			wantFiles: nil,
		},
		{
			name:      "bundled short flags",
			args:      []string{"-lw", "file.txt"},
			wantOpts:  options{lines: true, words: true},
			wantFiles: []string{"file.txt"},
		},
		{
			name:      "long flags",
			args:      []string{"--lines", "--bytes"},
			wantOpts:  options{lines: true, bytes: true},
			wantFiles: nil,
		},
		{
			name:      "dash means stdin",
			args:      []string{"-l", "-"},
			wantOpts:  options{lines: true},
			wantFiles: []string{"-"},
		},
		{
			name:      "double dash stops flags",
			args:      []string{"-l", "--", "-weird-name"},
			wantOpts:  options{lines: true},
			wantFiles: []string{"-weird-name"},
		},
		{
			name:    "unknown short flag",
			args:    []string{"-x"},
			wantErr: true,
		},
		{
			name:    "unknown long flag",
			args:    []string{"--nope"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, files, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseArgs(%v) expected error, got nil", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%v) unexpected error: %v", tt.args, err)
			}
			if opts != tt.wantOpts {
				t.Errorf("opts = %+v, want %+v", opts, tt.wantOpts)
			}
			if !equalSlices(files, tt.wantFiles) {
				t.Errorf("files = %v, want %v", files, tt.wantFiles)
			}
		})
	}
}

// TestFormatLine confirms columns are right-aligned and the filename appended.
func TestFormatLine(t *testing.T) {
	c := counts{lines: 7, words: 58, chars: 339, bytes: 339}

	// Default columns (lines, words, bytes) width 3, with a filename.
	got := formatLine(c, "test.txt", options{lines: true, words: true, bytes: true}, 3)
	want := "  7  58 339 test.txt"
	if got != want {
		t.Errorf("formatLine = %q, want %q", got, want)
	}

	// stdin (no name) should have no trailing filename.
	got = formatLine(c, "", options{lines: true}, 3)
	want = "  7"
	if got != want {
		t.Errorf("formatLine stdin = %q, want %q", got, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
