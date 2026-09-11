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

// EditKeys is the key sequence sent to Neovim to open path at line (1-based;
// 0 leaves the cursor alone): drop to normal mode from whatever mode the
// editor is in, then `:e +line ./path`.
func EditKeys(path string, line int) string {
	at := ""
	if line > 0 {
		at = fmt.Sprintf("+%d ", line)
	}
	return `<C-\><C-n>:e ` + at + escapeCmdArg(RelPath(path)) + "<CR>"
}

// escapeCmdArg backslash-escapes the characters that would otherwise be
// special in an Ex command argument (like Neovim's fnameescape()).
func escapeCmdArg(s string) string {
	const special = " \t\"#%*[|`\\"
	var sb strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// Open asks the Neovim listening on the PR's socket to edit path, placing
// the cursor on line (1-based; 0 leaves it alone), then, when running inside
// tmux, switches the current session to the editor window.
func Open(id string, path string, line int) error {
	if out, err := execCommand("nvim", "--server", SockPath(id), "--remote-send", EditKeys(path, line)).CombinedOutput(); err != nil {
		return fmt.Errorf("nvim --remote-send: %s", firstLine(out, err))
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
