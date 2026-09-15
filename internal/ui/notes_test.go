package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/diff"
	"ghpr/internal/state"
)

func TestLineNotes(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
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
	// A is empty at first; a off a diff line is refused.
	press("A")
	if m.screen != screenDiff || !strings.Contains(m.status, "No notes yet") {
		t.Fatalf("A with no marks: status=%q", m.status)
	}
	m.cursor = 0 // hunk header
	press("a")
	if m.overlay != overlayNone || !m.statusErr {
		t.Fatal("a on a hunk header should be refused")
	}
	// a on row 6 (+"fmt", new line 4) asks for a note; an empty note is
	// refused, esc cancels without a mark.
	m.cursor = 6
	press("a")
	if m.overlay != overlayNote || m.ntPending.newNum != 4 || m.ntPending.path != "main.go" {
		t.Fatalf("prompt: overlay=%v pending=%+v", m.overlay, m.ntPending)
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "⚑ Add note at main.go:4:") {
		t.Fatalf("prompt missing:\n%s", plain)
	}
	press("enter")
	if m.overlay != overlayNote || !strings.Contains(m.status, "Type the note first") {
		t.Fatal("empty note should be refused")
	}
	typeText("x")
	press("esc")
	if m.overlay != overlayNone || len(m.notes) != 0 {
		t.Fatal("esc cancels")
	}
	// Save a mark with a note; the gutter turns amber and the status shows it.
	press("a")
	typeText("check impo")
	press("backspace")
	typeText("ort order")
	press("enter")
	if len(m.notes) != 1 || m.notes[0].body != "check import order" || m.notes[0].newNum != 4 || m.notes[0].text != `    "fmt"` {
		t.Fatalf("marks = %+v", m.notes)
	}
	if m.rowNoteIndex(6) != 0 || m.rowNoteIndex(5) != -1 {
		t.Fatal("row lookup")
	}
	m.status = "" // the transient "Note added …" message takes precedence
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "⚑ check import order") {
		t.Fatalf("status should show the note:\n%s", plain)
	}
	m.cursor = 5
	if strings.Contains(ansi.Strip(m.View().Content), "⚑ check import order") {
		t.Fatal("note only shows on the noted row")
	}
	// A mark on a removed line is keyed by its old number; split mode still
	// finds both marks.
	m.cursor = 3 // -import "fmt", old line 3
	press("a")
	typeText("why removed?")
	press("enter")
	if len(m.notes) != 2 || m.notes[1].newNum != 0 || m.notes[1].oldNum != 3 {
		t.Fatalf("del mark = %+v", m.notes[1])
	}
	press("s") // side-by-side
	found := 0
	for i := range m.rows {
		if m.rowNoteIndex(i) >= 0 {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("split mode should show both marks, found %d", found)
	}
	press("s")
	// A lists marks; l jumps, h returns, d removes, esc closes.
	m.cursor = 0
	press("A")
	if m.screen != screenNotes || len(m.notes) != 2 {
		t.Fatalf("screen=%v", m.screen)
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Notes (2)", "main.go:4", "check import order", `+     "fmt"`, "main.go:-3", "why removed?", `- import "fmt"`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in:\n%s", want, plain)
		}
	}
	press("j")
	press("l")
	if m.screen != screenDiff || m.backTo != screenNotes || m.cursor != 3 {
		t.Fatalf("l should jump to the removed line: screen=%v cursor=%d", m.screen, m.cursor)
	}
	press("h")
	if m.screen != screenNotes || m.ntIdx != 1 {
		t.Fatalf("h returns to the marks: screen=%v idx=%d", m.screen, m.ntIdx)
	}
	press("k")
	press("enter")
	if m.cursor != 6 {
		t.Fatalf("enter should jump to row 6: %d", m.cursor)
	}
	// Opening A while on a noted row preselects it; d removes it.
	press("A")
	if m.ntIdx != 0 {
		t.Fatalf("preselect: idx=%d", m.ntIdx)
	}
	press("d")
	if len(m.notes) != 1 || m.notes[0].body != "why removed?" || m.screen != screenNotes {
		t.Fatalf("d should remove the first mark: %+v", m.notes)
	}
	press("esc")
	if m.screen != screenDiff || m.hasBack() {
		t.Fatal("esc closes the notes screen")
	}
	// a on a marked line removes it; removing the last mark from the list
	// returns to the diff.
	m.cursor = 3
	press("a")
	if len(m.notes) != 0 || !strings.Contains(m.status, "Note removed") {
		t.Fatalf("toggle off: %+v %q", m.notes, m.status)
	}
	m.cursor = 6
	press("a")
	typeText("n")
	press("enter")
	press("A")
	press("d")
	if m.screen != screenDiff || len(m.notes) != 0 {
		t.Fatal("removing the last mark leaves the list")
	}
	// Mouse in the notes screen: click selects, second click opens.
	m.cursor = 6
	press("a")
	typeText("one")
	press("enter")
	m.cursor = 11
	press("a")
	typeText("two")
	press("enter")
	m.cursor = 0
	press("A")
	y := headerH + paneHeaderH + ntItemH + 1
	m.Update(tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
	if m.ntIdx != 1 || m.screen != screenNotes {
		t.Fatalf("click selects: idx=%d", m.ntIdx)
	}
	m.Update(tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
	if m.screen != screenDiff || m.cursor != 11 {
		t.Fatalf("second click opens: screen=%v cursor=%d", m.screen, m.cursor)
	}
	// Leaving for the PR list forgets the marks.
	m.backToList()
	if len(m.notes) != 0 {
		t.Fatal("marks are per PR session")
	}
}

func TestNotesPersist(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t)
	m.SetStore(st)
	m.loadNotes()
	press := func(k string) {
		msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		if k == "enter" {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		}
		m.handleKey(msg)
	}
	m.cursor = 6
	press("a")
	for _, r := range "remember me" {
		press(string(r))
	}
	press("enter")
	m.cursor = 3
	press("a")
	for _, r := range "and me" {
		press(string(r))
	}
	press("enter")
	if got := st.GetNotes(m.prKey()); len(got) != 2 || got[0].Body != "remember me" || got[0].NewLine != 4 || got[1].OldLine != 3 || got[1].Kind != "del" {
		t.Fatalf("store should hold both notes: %+v", got)
	}
	// A fresh model on the same store sees them once the diff has loaded,
	// on the same rows.
	st2, _ := state.Open(dir)
	m2 := newTestModel(t)
	m2.SetStore(st2)
	m2.pending = 2
	m2.Update(diffMsg{files: diff.Parse(sample)})
	if len(m2.notes) != 2 || m2.notes[0].body != "remember me" || m2.notes[0].kind != diff.Add || m2.rowNoteIndex(6) != 0 || m2.rowNoteIndex(3) != 1 {
		t.Fatalf("reloaded notes: %+v", m2.notes)
	}
	// Removing one in the second model is persisted too.
	m2.cursor = 6
	_, _ = m2.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	st3, _ := state.Open(dir)
	if got := st3.GetNotes(m2.prKey()); len(got) != 1 || got[0].Body != "and me" {
		t.Fatalf("after removal: %+v", got)
	}
	// Without a store notes still work for the session.
	m3 := newTestModel(t)
	m3.cursor = 6
	_, _ = m3.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	_, _ = m3.handleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	_, _ = m3.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m3.notes) != 1 || m3.saveNotes() != nil {
		t.Fatal("no store: in-memory only")
	}
}
