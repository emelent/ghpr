package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/gh"
)

func TestThreadsOnly(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "ctrl+p":
			msg = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
		case "ctrl+/":
			msg = tea.KeyPressMsg{Code: '/', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	// Both files carry threads in the fixture: the filter hides nothing.
	press("T")
	if !m.threadsOnly || len(m.shownFiles()) != 2 {
		t.Fatalf("threadsOnly=%v shown=%v", m.threadsOnly, m.shownFiles())
	}
	// Drop b.py's thread: it disappears from the list, the tree, ] / [,
	// the file picker and the all-files search.
	mainOnly := []gh.Thread{m.threads[0], m.threads[1]}
	m.Update(threadsMsg{threads: mainOnly})
	if shown := m.shownFiles(); len(shown) != 1 || shown[0] != 0 {
		t.Fatalf("shown=%v", shown)
	}
	for _, n := range m.treeNodes {
		if !n.isDir && m.files[n.fileIdx].Path() == "b.py" {
			t.Fatal("b.py should be out of the tree")
		}
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Files (1 of 2)") || !strings.Contains(plain, "with comments") {
		t.Fatalf("title should say 1 of 2 with comments:\n%s", plain)
	}
	press("]")
	if m.fileIdx != 0 {
		t.Fatalf("] must not reach a hidden file: fileIdx=%d", m.fileIdx)
	}
	m.tree = false
	if plain := ansi.Strip(m.View().Content); strings.Contains(plain, "b.py") {
		t.Fatalf("flat list should hide b.py:\n%s", plain)
	}
	m.filesFocused = true
	press("j")
	press("G")
	if m.fileIdx != 0 {
		t.Fatalf("flat list keys must stay on shown files: fileIdx=%d", m.fileIdx)
	}
	m.filesFocused = false
	press("ctrl+p")
	if len(m.fpResults) != 1 || m.fpResults[0].fileIdx != 0 {
		t.Fatalf("picker: %+v", m.fpResults)
	}
	press("esc")
	press("ctrl+/")
	for _, r := range "return" {
		press(string(r))
	}
	if len(m.gsResults) != 0 {
		t.Fatalf("global search should skip hidden files: %+v", m.gsResults)
	}
	press("esc")
	// Off again: everything is back.
	press("T")
	if m.threadsOnly || len(m.shownFiles()) != 2 {
		t.Fatal("second T turns the filter off")
	}
	// Turning it on while on a file without threads moves to one that has.
	m.selectFile(1)
	press("T")
	if !m.threadsOnly || m.fileIdx != 0 {
		t.Fatalf("T on b.py should jump to main.go: fileIdx=%d", m.fileIdx)
	}
	press("T")
	// With no threads at all the filter is refused.
	m.Update(threadsMsg{threads: nil})
	press("T")
	if m.threadsOnly || !m.statusErr || !strings.Contains(m.status, "No files with comments") {
		t.Fatalf("expected refusal: threadsOnly=%v status=%q", m.threadsOnly, m.status)
	}
}
