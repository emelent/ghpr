package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// stubClipboard captures what the platform clipboard tool would receive.
func stubClipboard(t *testing.T) *string {
	t.Helper()
	got := new(string)
	prev := clipboardWrite
	clipboardWrite = func(s string) error { *got = s; return nil }
	t.Cleanup(func() { clipboardWrite = prev })
	return got
}

func TestCopySelection(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)

	// rows: 0 hunk, 3 del `import "fmt"`, 5 add `import (`, 6 add "\t\"fmt\"", 7 thread T1
	m.cursor = 5
	m.toggleSelection()
	m.cursor = 6
	cmd := m.handleKeyCmd(t, 'y')
	if cmd == nil {
		t.Fatal("y should return a command (OSC 52 + status)")
	}
	if want := "import (\n    \"fmt\""; *got != want { // tabs are shown, and copied, as four spaces
		t.Fatalf("clipboard = %q, want %q", *got, want)
	}
	if m.selecting {
		t.Fatal("copying should drop the selection")
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "Copied 2 line(s)") {
		t.Fatalf("status bar should confirm the copy:\n%s", plain)
	}
}

func TestCopyCursorLineWithoutSelection(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)

	m.cursor = 3 // the removed `import "fmt"`: the marker is not copied
	m.handleKeyCmd(t, 'y')
	if *got != `import "fmt"` {
		t.Fatalf("clipboard = %q", *got)
	}
}

func TestCopyThreadAndHunkRows(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)

	m.cursor = 7 // thread T1: alice then bob
	m.handleKeyCmd(t, 'y')
	want := "@alice: Why the parens?\nSecond line that is fairly long and should wrap somewhere around the pane width hopefully yes.\n@bob: Style."
	if *got != want {
		t.Fatalf("thread copy = %q, want %q", *got, want)
	}

	// A range starting at the hunk header keeps the header verbatim.
	m.cursor = 0
	m.toggleSelection()
	m.cursor = 1
	m.handleKeyCmd(t, 'y')
	if !strings.HasPrefix(*got, "@@ -1,6 +1,8 @@") || !strings.HasSuffix(*got, "\npackage main") {
		t.Fatalf("hunk copy = %q", *got)
	}
}

func TestCopySideBySidePrefersNewSide(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)
	m.split = true
	m.rebuildRows()

	var pair int
	for i := range m.rows {
		if r := &m.rows[i]; r.kind == rowSplit && r.left != nil && r.right != nil {
			pair = i
			break
		}
	}
	if pair == 0 {
		t.Skip("no left+right pair in the fixture")
	}
	m.cursor = pair
	m.handleKeyCmd(t, 'y')
	if *got != m.rows[pair].right.Text {
		t.Fatalf("split copy = %q, want the new side %q", *got, m.rows[pair].right.Text)
	}
}

func TestCopyMouseSelection(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)
	m.View() // lay out rowStart
	x := m.filesWidth() + 5
	rowY := func(i int) int { return headerH + paneHeaderH + m.rowStart[i] - m.scroll }

	m.Update(tea.MouseClickMsg{X: x, Y: rowY(5), Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: x, Y: rowY(6), Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: x, Y: rowY(6), Button: tea.MouseLeft})
	if lo, hi, ok := m.selection(); !ok || lo != 5 || hi != 6 {
		t.Fatalf("drag selection: %d-%d ok=%v", lo, hi, ok)
	}
	m.handleKeyCmd(t, 'y')
	if want := "import (\n    \"fmt\""; *got != want { // tabs are shown, and copied, as four spaces
		t.Fatalf("clipboard = %q, want %q", *got, want)
	}
}

func TestCopyEmptyDiff(t *testing.T) {
	m := newTestModel(t)
	got := stubClipboard(t)
	m.rows = nil
	m.handleKeyCmd(t, 'y')
	if *got != "" {
		t.Fatalf("nothing to copy should leave the clipboard alone, got %q", *got)
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "Nothing to copy") {
		t.Fatalf("expected an error status:\n%s", plain)
	}
}

// handleKeyCmd presses a single rune and returns the resulting command.
func (m *Model) handleKeyCmd(t *testing.T, r rune) tea.Cmd {
	t.Helper()
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	return cmd
}
