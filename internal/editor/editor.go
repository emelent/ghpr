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

// EnvSock names the environment variable that overrides the socket path
// derived from the pull request's node id.
const EnvSock = "NVIM_SOCK"

// execCommand builds the commands run by Open; tests replace it.
var execCommand = exec.Command

// SockPath is the Neovim server socket ghpr looks for when reviewing the
// pull request with GraphQL node id (e.g. PR_kwDOPRY-OM8AAAABCBHOJc): start
// Neovim with `nvim --listen /tmp/nvim.<id>.sock` in the repository. $NVIM_SOCK
// overrides that path, so one Neovim can serve whichever PR is being reviewed.
func SockPath(id string) string {
	if s := os.Getenv(EnvSock); s != "" {
		return s
	}
	return fmt.Sprintf("/tmp/nvim.%s.sock", id)
}

// HasServer reports whether a Neovim server socket exists for the PR with
// node id (or at $NVIM_SOCK).
func HasServer(id string) bool {
	if id == "" && os.Getenv(EnvSock) == "" {
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

// EditExpr is the Vimscript expression evaluated on the server to open path
// at line (1-based; 0 leaves the cursor alone). An expression, unlike keys
// sent with --remote-send, is not subject to the user's mappings (plugins
// binding <C-n> or ':' would otherwise swallow the command) and works from
// any mode. fnameescape runs on the server so odd file names are handled.
func EditExpr(path string, line int) string {
	at := ""
	if line > 0 {
		at = fmt.Sprintf("+%d ", line)
	}
	return fmt.Sprintf("execute('edit %s' . fnameescape('%s'))", at, strings.ReplaceAll(RelPath(path), "'", "''"))
}

// Open asks the Neovim listening on the PR's socket to edit path, placing
// the cursor on line (1-based; 0 leaves it alone), then, when running inside
// tmux, switches the current session to the editor window.
func Open(id string, path string, line int) error {
	if out, err := execCommand("nvim", "--server", SockPath(id), "--remote-expr", EditExpr(path, line)).CombinedOutput(); err != nil {
		return fmt.Errorf("nvim --remote-expr: %s", firstLine(out, err))
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
