package main

import "testing"

// Table-driven unit tests for the subject matcher — the conceptual core of the
// broker, tested in complete isolation from any networking.
func TestMatchSubject(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		subject string
		want    bool
	}{
		{"exact match", "foo.bar", "foo.bar", true},
		{"exact mismatch", "foo.bar", "foo.baz", false},
		{"different length no wildcard", "foo", "foo.bar", false},
		{"longer pattern than subject", "foo.bar", "foo", false},

		{"star matches one token", "foo.*", "foo.bar", true},
		{"star does not match two tokens", "foo.*", "foo.bar.baz", false},
		{"star does not match zero tokens", "foo.*", "foo", false},
		{"star in middle", "foo.*.baz", "foo.bar.baz", true},
		{"star in middle mismatch tail", "foo.*.baz", "foo.bar.qux", false},
		{"leading star", "*.bar", "foo.bar", true},

		{"tail matches one trailing token", "foo.>", "foo.bar", true},
		{"tail matches many trailing tokens", "foo.>", "foo.bar.baz.qux", true},
		{"tail requires at least one token", "foo.>", "foo", false},
		{"bare tail matches everything nonempty", ">", "anything", true},
		{"bare tail matches deep subject", ">", "a.b.c", true},

		{"star then tail", "foo.*.>", "foo.bar.baz.qux", true},
		{"star then tail too short", "foo.*.>", "foo.bar", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchSubject(tc.pattern, tc.subject); got != tc.want {
				t.Errorf("matchSubject(%q, %q) = %v, want %v",
					tc.pattern, tc.subject, got, tc.want)
			}
		})
	}
}
