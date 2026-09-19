package editor

import (
	"os"
	"os/exec"
	"path/filepath"
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
	// $NVIM_SOCK wins over the id-derived path.
	sock := filepath.Join(t.TempDir(), "nvim.sock")
	t.Setenv(EnvSock, sock)
	if got := SockPath("PR_abc"); got != sock {
		t.Fatalf("SockPath with %s = %q, want %q", EnvSock, got, sock)
	}
	if HasServer("PR_abc") || HasServer("") {
		t.Fatalf("no socket exists at %s yet", sock)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !HasServer("") {
		t.Fatalf("%s should be used even without a PR id", EnvSock)
	}
	if got, want := EditExpr("it's here/a b.txt", 7), "execute('edit +7 ' . fnameescape('./it''s here/a b.txt'))"; got != want {
		t.Fatalf("EditExpr = %q, want %q", got, want)
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
	t.Setenv(EnvSock, "")
	if err := Open("PR_abc", "my-change.txt", 0); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"nvim", "--server", "/tmp/nvim.PR_abc.sock", "--remote-expr", "execute('edit ' . fnameescape('./my-change.txt'))"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	// Inside tmux the editor window of the current session is selected too.
	calls = nil
	t.Setenv("TMUX", "/tmp/tmux-501/default,1,0")
	if err := Open("PR_abc", "my-change.txt", 42); err != nil {
		t.Fatal(err)
	}
	want = [][]string{{"nvim", "--server", "/tmp/nvim.PR_abc.sock", "--remote-expr", "execute('edit +42 ' . fnameescape('./my-change.txt'))"}, {"tmux", "select-window", "-t", ":code"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	// $NVIM_SOCK replaces the id-derived socket.
	calls = nil
	t.Setenv("TMUX", "")
	t.Setenv(EnvSock, "/run/nvim.sock")
	if err := Open("PR_abc", "my-change.txt", 0); err != nil {
		t.Fatal(err)
	}
	want = [][]string{{"nvim", "--server", "/run/nvim.sock", "--remote-expr", "execute('edit ' . fnameescape('./my-change.txt'))"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	t.Setenv(EnvSock, "")

	// A failing nvim surfaces its message and stops before tmux.
	calls, fail = nil, true
	err := Open("PR_abc", "x", 1)
	if err == nil || err.Error() != "nvim --remote-expr: E247: no server" || len(calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, calls)
	}
}
