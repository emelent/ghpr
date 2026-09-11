package editor

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestPaths(t *testing.T) {
	if got := SockPath("PR_kwDOPRY-OM8AAAABCBHOJc"); got != "/tmp/nvim.PR_kwDOPRY-OM8AAAABCBHOJc.sock" {
		t.Fatalf("SockPath = %q", got)
	}
	for in, want := range map[string]string{"a/b.go": "./a/b.go", "./a/b.go": "./a/b.go", "/a/b.go": "./a/b.go"} {
		if got := RelPath(in); got != want {
			t.Errorf("RelPath(%q) = %q, want %q", in, got, want)
		}
	}
	if HasServer("PR_does_not_exist") || HasServer("") {
		t.Fatal("no socket should exist for an unknown or empty id")
	}
	if got, want := EditKeys("dir with space/a#b.txt", 7), `<C-\><C-n>:e +7 ./dir\ with\ space/a\#b.txt<CR>`; got != want {
		t.Fatalf("EditKeys = %q, want %q", got, want)
	}
}

func TestOpen(t *testing.T) {
	var calls [][]string
	fail := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		if fail {
			return exec.Command("sh", "-c", "echo 'E247: no server' >&2; exit 1")
		}
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = exec.Command })

	// Outside tmux only Neovim is called.
	t.Setenv("TMUX", "")
	if err := Open("PR_abc", "my-change.txt", 0); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"nvim", "--server", "/tmp/nvim.PR_abc.sock", "--remote-send", `<C-\><C-n>:e ./my-change.txt<CR>`}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	// Inside tmux the editor window of the current session is selected too.
	calls = nil
	t.Setenv("TMUX", "/tmp/tmux-501/default,1,0")
	if err := Open("PR_abc", "my-change.txt", 42); err != nil {
		t.Fatal(err)
	}
	want = [][]string{{"nvim", "--server", "/tmp/nvim.PR_abc.sock", "--remote-send", `<C-\><C-n>:e +42 ./my-change.txt<CR>`}, {"tmux", "select-window", "-t", ":code"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	// A failing nvim surfaces its message and stops before tmux.
	calls, fail = nil, true
	err := Open("PR_abc", "x", 1)
	if err == nil || err.Error() != "nvim --remote-send: E247: no server" || len(calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
}
