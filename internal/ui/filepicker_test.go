package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFilePicker(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+p", "ctrl+j", "ctrl+k":
			msg = tea.KeyPressMsg{Code: rune(k[len(k)-1]), Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	typeText := func(s string) {
		for _, r := range s {
			press(string(r))
		}
	}
	// Files: 0 main.go, 1 b.py. ctrl+p lists everything.
	press("ctrl+p")
	if m.overlay != overlayFiles || len(m.fpResults) != 2 || m.fpIdx != 0 {
		t.Fatalf("picker should open with every file: overlay=%v results=%d", m.overlay, len(m.fpResults))
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Go to file") || !strings.Contains(plain, "2 of 2") || !strings.Contains(plain, "b.py") {
		t.Fatalf("picker view missing:\n%s", plain)
	}
	// Arrow / ctrl keys move; enter opens the highlighted file.
	press("down")
	press("ctrl+k")
	press("ctrl+j")
	if m.fpIdx != 1 {
		t.Fatalf("navigation: idx=%d", m.fpIdx)
	}
	press("enter")
	if m.overlay != overlayNone || m.fileIdx != 1 || m.filesFocused {
		t.Fatalf("enter should open b.py: overlay=%v fileIdx=%d", m.overlay, m.fileIdx)
	}
	// Fuzzy: "mg" matches main.go only; spaces are ignored.
	press("ctrl+p")
	typeText("m g")
	if len(m.fpResults) != 1 || m.files[m.fpResults[0].fileIdx].Path() != "main.go" || len(m.fpResults[0].matched) != 2 {
		t.Fatalf("fuzzy filter: %+v", m.fpResults)
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "> m g") || !strings.Contains(plain, "1 of 2") {
		t.Fatalf("query and count missing:\n%s", plain)
	}
	// No hit: enter is a no-op and the picker stays open.
	typeText("zz")
	press("enter")
	if m.overlay != overlayFiles || len(m.fpResults) != 0 || !strings.Contains(ansi.Strip(m.View().Content), "No file matches") {
		t.Fatalf("no-match state: overlay=%v results=%d", m.overlay, len(m.fpResults))
	}
	press("backspace")
	press("backspace")
	press("enter")
	if m.overlay != overlayNone || m.fileIdx != 0 {
		t.Fatalf("backspace then enter should open main.go: fileIdx=%d", m.fileIdx)
	}
	// esc cancels without changing the file.
	press("ctrl+p")
	typeText("b")
	press("esc")
	if m.overlay != overlayNone || m.fileIdx != 0 {
		t.Fatal("esc should cancel")
	}
	// / opens the picker only while the file list is focused.
	m.filesFocused = true
	press("/")
	if m.overlay != overlayFiles {
		t.Fatal("/ in the file list should open the picker")
	}
	press("ctrl+p")
	m.filesFocused = false
	press("/")
	if m.overlay != overlaySearch {
		t.Fatal("/ in the diff should open the search")
	}
}
