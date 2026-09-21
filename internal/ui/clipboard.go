package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// clipboardWrite hands text to the platform clipboard tool (pbcopy, xclip,
// wl-copy…). It is a variable so tests can stub it.
var clipboardWrite = clipboard.WriteAll

// copyToClipboard puts s on the clipboard two ways: through the platform
// tool, and through OSC 52, which is what reaches the local machine when
// ghpr runs over SSH. Terminals honour one or the other (Terminal.app has
// no OSC 52; tmux needs set-clipboard on), so both always run. The platform
// tool's error is dropped: a missing xclip should not fail the copy that
// OSC 52 may still have made.
func copyToClipboard(s string) tea.Cmd {
	if !clipboard.Unsupported {
		_ = clipboardWrite(s)
	}
	return tea.SetClipboard(s)
}

// yank copies the selected rows — or the row under the cursor when nothing
// is selected — to the clipboard, then drops the selection the way vim's
// visual-mode yank does.
func (m *Model) yank() tea.Cmd {
	lo, hi, ok := m.selection()
	if !ok {
		lo, hi = m.cursor, m.cursor
	}
	text := m.rowsText(lo, hi)
	if text == "" {
		return m.setStatus("Nothing to copy", true)
	}
	m.clearSelection()
	n := strings.Count(text, "\n") + 1
	return tea.Batch(copyToClipboard(text), m.setStatus(fmt.Sprintf("Copied %d line(s)", n), false))
}

// rowsText renders rows [lo, hi] as plain text for the clipboard: diff
// lines without their +/- marker so the result pastes as code, hunk headers
// verbatim, review threads as "@author: body" and notes behind their flag.
// A side-by-side pair yields its new side when it has one, so what lands on
// the clipboard is the code after the change. Indentation is whatever the
// pane shows: HighlightFile has already turned tabs into four spaces.
func (m *Model) rowsText(lo, hi int) string {
	if lo > hi {
		lo, hi = hi, lo
	}
	var out []string
	for i := max(0, lo); i <= hi && i < len(m.rows); i++ {
		r := &m.rows[i]
		switch r.kind {
		case rowHunk:
			out = append(out, r.hunk.Header)
		case rowLine:
			out = append(out, r.line.Text)
		case rowSplit:
			switch {
			case r.right != nil:
				out = append(out, r.right.Text)
			case r.left != nil:
				out = append(out, r.left.Text)
			}
		case rowThread:
			for _, c := range r.thread.Comments {
				body := strings.ReplaceAll(strings.TrimRight(c.Body, "\n"), "\r", "")
				out = append(out, "@"+c.Author+": "+body)
			}
		case rowNote:
			out = append(out, "⚑ "+r.note)
		}
	}
	return strings.Join(out, "\n")
}
