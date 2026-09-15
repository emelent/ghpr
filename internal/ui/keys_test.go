package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/keys"
)

func TestCustomKeys(t *testing.T) {
	m := newTestModel(t)
	km := keys.Default()
	if err := km.Apply(`
[diff]
next_file = ["n"]
next = ["ctrl+n"]
down = ["ctrl+j"]
notes = []
help = ["f1"]

[prompt]
down = ["ctrl+d"]

[comments]
close = ["x"]
`); err != nil {
		t.Fatal(err)
	}
	m.SetKeys(km, "/home/me/.config/ghpr/keys.toml")
	press := func(k string) {
		msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		switch k {
		case "ctrl+n", "ctrl+j", "ctrl+d":
			msg = tea.KeyPressMsg{Code: rune(k[len(k)-1]), Mod: tea.ModCtrl}
		case "f1":
			msg = tea.KeyPressMsg{Code: tea.KeyF1}
		}
		m.handleKey(msg)
	}
	// n now steps files; ] does nothing; the old j is unbound, ctrl+j moves.
	press("n")
	if m.fileIdx != 1 {
		t.Fatalf("n should go to the next file: %d", m.fileIdx)
	}
	press("[")
	if m.fileIdx != 0 {
		t.Fatalf("[ keeps its default: %d", m.fileIdx)
	}
	press("n")
	press("]")
	if m.fileIdx != 1 {
		t.Fatalf("] is unbound now: %d", m.fileIdx)
	}
	m.selectFile(0)
	m.cursor = 0
	press("j")
	if m.cursor != 0 {
		t.Fatal("j was rebound away from down")
	}
	press("ctrl+j")
	if m.cursor != 1 {
		t.Fatalf("ctrl+j should move down: %d", m.cursor)
	}
	// ctrl+n jumps to the next thread (the old n).
	press("ctrl+n")
	if r := m.currentRow(); r == nil || r.kind != rowThread {
		t.Fatalf("ctrl+n should jump to a thread: %d", m.cursor)
	}
	// Unbound action.
	press("A")
	if m.screen != screenDiff {
		t.Fatal("A should be unbound")
	}
	// Help opens on F1 and reflects the bindings and their source.
	press("f1")
	if m.overlay != overlayHelp {
		t.Fatal("f1 should open help")
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"/home/me/.config/ghpr/keys.toml", "n / [", "ctrl+n / N", "(unbound)", "ctrl+j / k/↑"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("help should show %q:\n%s", want, plain)
		}
	}
	press("f1")
	// Prompt context: ctrl+d moves down in the file picker.
	m.handleKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if m.overlay != overlayFiles {
		t.Fatal("ctrl+p keeps its default")
	}
	press("ctrl+d")
	if m.fpIdx != 1 {
		t.Fatalf("ctrl+d should move down in the picker: %d", m.fpIdx)
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.fpIdx != 1 {
		t.Fatal("down is unbound in the prompt now")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) // the opening key still closes it
	if m.overlay != overlayNone {
		t.Fatal("ctrl+p should close the picker")
	}
	// Comments screen: x closes, esc no longer does.
	press("i")
	if m.screen != screenComments {
		t.Fatal("i opens comments")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != screenComments {
		t.Fatal("esc is unbound in comments")
	}
	press("x")
	if m.screen != screenDiff {
		t.Fatal("x should close comments")
	}
	// Status hints use the map too (a wider window: the rebound names are
	// longer and the bar hides hints that do not fit).
	m.filesFocused = false
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "ctrl+j/k move") {
		t.Fatalf("hints should show the rebound key:\n%s", plain)
	}
}
