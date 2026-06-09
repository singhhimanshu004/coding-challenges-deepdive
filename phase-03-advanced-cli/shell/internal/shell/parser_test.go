package shell

import "testing"

// ---- PARSER tests: pipelines, redirections, sequencing -------------------

func mustParse(t *testing.T, in string) *List {
	t.Helper()
	toks, err := tokenize(in)
	if err != nil {
		t.Fatalf("tokenize(%q): %v", in, err)
	}
	l, err := parse(toks)
	if err != nil {
		t.Fatalf("parse(%q): %v", in, err)
	}
	return l
}

func TestParsePipeline(t *testing.T) {
	l := mustParse(t, "echo hi | cat | wc -l")
	if len(l.Items) != 1 {
		t.Fatalf("want 1 list item, got %d", len(l.Items))
	}
	pl := l.Items[0].Pipelines[0]
	if len(pl.Cmds) != 3 {
		t.Fatalf("want 3 pipeline stages, got %d", len(pl.Cmds))
	}
	if pl.Cmds[0].Args[0].raw() != "echo" || pl.Cmds[2].Args[0].raw() != "wc" {
		t.Errorf("unexpected stage commands: %v", pl.Cmds)
	}
}

func TestParseRedirections(t *testing.T) {
	l := mustParse(t, "sort < in.txt > out.txt 2> err.txt")
	cmd := l.Items[0].Pipelines[0].Cmds[0]
	if len(cmd.Redirs) != 3 {
		t.Fatalf("want 3 redirects, got %d", len(cmd.Redirs))
	}
	checks := []struct {
		fd   int
		mode string
		file string
	}{
		{0, "in", "in.txt"},
		{1, "out", "out.txt"},
		{2, "out", "err.txt"},
	}
	for i, c := range checks {
		r := cmd.Redirs[i]
		if r.fd != c.fd || r.mode != c.mode || r.word.raw() != c.file {
			t.Errorf("redir %d = {%d %s %s}, want %v", i, r.fd, r.mode, r.word.raw(), c)
		}
	}
}

func TestParseAppendRedirect(t *testing.T) {
	cmd := mustParse(t, "echo x >> log").Items[0].Pipelines[0].Cmds[0]
	if len(cmd.Redirs) != 1 || cmd.Redirs[0].mode != "append" {
		t.Fatalf("expected append redirect, got %+v", cmd.Redirs)
	}
}

func TestParseSequencing(t *testing.T) {
	l := mustParse(t, "echo a ; echo b ; echo c")
	if len(l.Items) != 3 {
		t.Fatalf("want 3 sequenced items, got %d", len(l.Items))
	}
}

func TestParseAndOr(t *testing.T) {
	ao := mustParse(t, "true && echo ok || echo no").Items[0]
	if len(ao.Pipelines) != 3 {
		t.Fatalf("want 3 pipelines in andor, got %d", len(ao.Pipelines))
	}
	if ao.Ops[0] != tAndAnd || ao.Ops[1] != tOrOr {
		t.Errorf("operators = %v, want [&& ||]", ao.Ops)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"| echo", "echo |", "echo >"} {
		toks, err := tokenize(in)
		if err != nil {
			continue // tokenizer rejection is also acceptable
		}
		if _, err := parse(toks); err == nil {
			t.Errorf("parse(%q): expected error, got none", in)
		}
	}
}
