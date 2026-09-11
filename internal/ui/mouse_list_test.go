package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/gh"
)

func TestMouseList(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 0, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(prListMsg{prs: []gh.PRSummary{{Number: 7, Title: "First"}, {Number: 8, Title: "Second"}, {Number: 9, Title: "Third"}}})
	if m.screen != screenPicker || m.list.Index() != 0 {
		t.Fatalf("picker should start on the first PR: screen=%v idx=%d", m.screen, m.list.Index())
	}
	click := func(y int) tea.Cmd {
		_, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		return cmd
	}
	// Find where the second PR is drawn and check it agrees with the geometry.
	y := -1
	for i, l := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(l, "#8  Second") {
			y = i
			break
		}
	}
	if y < 0 {
		t.Fatalf("second PR not rendered:\n%s", ansi.Strip(m.View().Content))
	}
	if want := m.listTopHeight() + m.listItemH + m.listItemGap; y != want {
		t.Fatalf("second PR drawn on line %d but the click math expects %d", y, want)
	}
	// Click selects; the description line of the same item counts too.
	click(y)
	if m.list.Index() != 1 || m.screen != screenPicker {
		t.Fatalf("click should select the second PR: idx=%d", m.list.Index())
	}
	click(y + m.listItemH + m.listItemGap + 1) // third PR's description line
	if m.list.Index() != 2 {
		t.Fatalf("click on a description line: idx=%d", m.list.Index())
	}
	// The blank line between items and the space below the list do nothing.
	click(y - 1)
	click(y + 20)
	click(0)
	if m.list.Index() != 2 {
		t.Fatalf("dead zones changed the selection: idx=%d", m.list.Index())
	}
	// Clicking the selected PR opens it.
	if cmd := click(y + m.listItemH + m.listItemGap); cmd == nil || m.screen != screenDiff || m.number != 9 || !m.fromPicker {
		t.Fatalf("second click should open PR 9: screen=%v number=%d", m.screen, m.number)
	}
	// Wheel moves the selection when back on the list.
	m.screen = screenPicker
	m.Update(tea.MouseWheelMsg{X: 4, Y: y, Button: tea.MouseWheelUp})
	if m.list.Index() != 1 {
		t.Fatalf("wheel up: idx=%d", m.list.Index())
	}
}
