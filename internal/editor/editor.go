// Package editor hands files from the review over to a running Neovim and
// brings its tmux window forward.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TmuxWindow is the window in the current tmux session that holds the editor.
const TmuxWindow = "code"

// execCommand builds the commands run by Open; tests replace it.
var execCommand = exec.Command

// SockPath is the Neovim server socket ghpr looks for when reviewing the
// pull request with GraphQL node id (e.g. PR_kwDOPRY-OM8AAAABCBHOJc): start
// Neovim with `nvim --listen /tmp/nvim.<id>.sock` in the repository.
func SockPath(id string) string { return fmt.Sprintf("/tmp/nvim.%s.sock", id) }

// HasServer reports whether a Neovim server socket exists for the PR with
// node id.
func HasServer(id string) bool {
	if id == "" {
		return false
	}
	_, err := os.Stat(SockPath(id))
	return err == nil
}

// RelPath turns a repository-relative path into the "./path" form given to
// Neovim, which resolves it against its working directory.
func RelPath(path string) string {
	return "./" + strings.TrimPrefix(strings.TrimPrefix(path, "./"), "/")
}

// Open asks the Neovim listening on the PR's socket to edit path, then, when
// running inside tmux, switches the current session to the editor window.
func Open(id string, path string) error {
	if out, err := execCommand("nvim", "--server", SockPath(id), "--remote", RelPath(path)).CombinedOutput(); err != nil {
		return fmt.Errorf("nvim --remote: %s", firstLine(out, err))
	}
	if os.Getenv("TMUX") == "" {
		return nil
	}
	// ":name" targets a window in the current session.
	if out, err := execCommand("tmux", "select-window", "-t", ":"+TmuxWindow).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux select-window %s: %s", TmuxWindow, firstLine(out, err))
	}
	return nil
}

func firstLine(out []byte, err error) string {
	if s := strings.TrimSpace(string(out)); s != "" {
		s, _, _ = strings.Cut(s, "\n")
		return s
	}
	return err.Error()
}
