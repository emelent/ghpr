package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsAreConsistent(t *testing.T) {
	m := Default()
	seen := map[string]bool{}
	for _, b := range Defaults {
		if len(b.Keys) == 0 || b.Desc == "" {
			t.Errorf("%s.%s needs keys and a description", b.Ctx, b.Action)
		}
		id := b.Ctx + "." + b.Action
		if seen[id] {
			t.Errorf("duplicate binding %s", id)
		}
		seen[id] = true
	}
	// Defaults must not bind one key to two actions in a context.
	if err := m.Apply(""); err != nil {
		t.Fatal(err)
	}
	if got := m.Translate(Diff, "j"); got != "j" {
		t.Fatalf("Translate default = %q", got)
	}
	if got := m.Translate(Diff, "down"); got != "j" {
		t.Fatalf("secondary key should map to the canonical one: %q", got)
	}
	if got := m.Translate(Prompt, "x"); got != "x" {
		t.Fatalf("unbound printable key passes through: %q", got)
	}
}

func TestApplyAndTranslate(t *testing.T) {
	m := Default()
	err := m.Apply(`
[diff]
next_file = ["n"]        # steal n from next
next = ["ctrl+n"]
quit = []                # unbind

[prompt]
down = "ctrl+d"
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Translate(Diff, "n"); got != "]" {
		t.Fatalf("n should now act as next_file: %q", got)
	}
	if got := m.Translate(Diff, "]"); got != Unbound {
		t.Fatalf("] is a default key with no action now: %q", got)
	}
	if got := m.Translate(Diff, "ctrl+n"); got != "n" {
		t.Fatalf("ctrl+n should act as next: %q", got)
	}
	if got := m.Translate(Diff, "q"); got != Unbound {
		t.Fatalf("unbound action: %q", got)
	}
	if got := m.Translate(Prompt, "ctrl+d"); got != "down" {
		t.Fatalf("single string binding: %q", got)
	}
	if !m.Is(Diff, "next_file", "n") || m.Is(Diff, "next", "n") {
		t.Fatal("Is")
	}
	if m.Label(Diff, "quit") != "(unbound)" || m.Label(Diff, "up") != "k/↑" || m.First(Diff, "next") != "ctrl+n" {
		t.Fatalf("labels: %q %q %q", m.Label(Diff, "quit"), m.Label(Diff, "up"), m.First(Diff, "next"))
	}
}

func TestApplyErrors(t *testing.T) {
	cases := map[string]string{
		"[nope]\nx = [\"a\"]":                          "unknown context",
		"[diff]\nnope = [\"a\"]":                       "unknown action",
		"[diff]\nquit = 3":                             "expected a key",
		"[diff]\nquit = [1]":                           "must be strings",
		"[diff]\nquit = [\"x\"]\nforce_quit = [\"x\"]": "bound to both",
		"[diff\n": "",
	}
	for src, want := range cases {
		err := Default().Apply(src)
		if err == nil || (want != "" && !strings.Contains(err.Error(), want)) {
			t.Errorf("Apply(%q) = %v, want %q", src, err, want)
		}
	}
}

func TestLoadAndDefaultTOML(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.toml")); err != nil {
		t.Fatalf("missing file should mean defaults: %v", err)
	}
	// The generated defaults file must load back unchanged.
	p := filepath.Join(dir, "keys.toml")
	if err := os.WriteFile(p, []byte(DefaultTOML()), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range Defaults {
		if got := strings.Join(m.Keys(b.Ctx, b.Action), ","); got != strings.Join(b.Keys, ",") {
			t.Fatalf("%s.%s round trip: %q", b.Ctx, b.Action, got)
		}
	}
	if err := os.WriteFile(p, []byte("[diff]\nquit = [\"q\"]\nforce_quit = [\"q\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("errors should name the file: %v", err)
	}
}
