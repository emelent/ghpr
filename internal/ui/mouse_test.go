package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMouse(t *testing.T) {
	m := newTestModel(t)
	m.View() // lay out rows and offsets
	x := m.filesWidth() + 5
	rowY := func(i int) int { return headerH + paneHeaderH + m.rowStart[i] - m.scroll }
	click := func(x, y int, mod tea.KeyMod) {
		m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: mod})
	}
	drag := func(x, y int) { m.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	release := func(x, y int) { m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}) }
	wheel := func(x int, b tea.MouseButton) { m.Update(tea.MouseWheelMsg{X: x, Y: 10, Button: b}) }

	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse reporting should be enabled")
	}
	// Click moves the cursor to the row under the pointer.
	m.filesFocused = true
	click(x, rowY(3), 0)
	release(x, rowY(3))
	if m.cursor != 3 || m.selecting || m.filesFocused {
		t.Fatalf("click: cursor=%d selecting=%v filesFocused=%v", m.cursor, m.selecting, m.filesFocused)
	}
	// Clicks on the pane header, the separator and the PR header do nothing.
	click(x, headerH, 0)
	click(m.filesWidth(), rowY(6), 0)
	click(x, 0, 0)
	if m.cursor != 3 {
		t.Fatalf("dead zones moved the cursor: %d", m.cursor)
	}
	// Drag selects from the pressed row to the pointer.
	click(x, rowY(5), 0)
	drag(x, rowY(6))
	drag(x, rowY(8))
	if lo, hi, ok := m.selection(); !ok || lo != 5 || hi != 8 || m.cursor != 8 {
		t.Fatalf("drag selection: %d-%d ok=%v cursor=%d", lo, hi, ok, m.cursor)
	}
	release(x, rowY(8))
	if _, _, ok := m.selection(); !ok || m.dragging {
		t.Fatalf("release should keep the selection and end the drag: ok=%v dragging=%v", ok, m.dragging)
	}
	// Dragging back to the origin leaves no selection.
	click(x, rowY(5), 0)
	drag(x, rowY(6))
	drag(x, rowY(5))
	release(x, rowY(5))
	if m.selecting || m.cursor != 5 {
		t.Fatalf("round-trip drag: selecting=%v cursor=%d", m.selecting, m.cursor)
	}
	// Dragging above the rows snaps to the first visible row.
	click(x, rowY(5), 0)
	drag(x, 0)
	if lo, hi, _ := m.selection(); lo != 0 || hi != 5 {
		t.Fatalf("drag above pane: %d-%d", lo, hi)
	}
	release(x, 0)
	m.clearSelection()
	// Shift+click extends from the cursor.
	click(x, rowY(3), 0)
	release(x, rowY(3))
	click(x, rowY(6), tea.ModShift)
	release(x, rowY(6))
	if lo, hi, ok := m.selection(); !ok || lo != 3 || hi != 6 {
		t.Fatalf("shift+click: %d-%d ok=%v", lo, hi, ok)
	}
	m.clearSelection()
	// Wheel moves the cursor by a few rows in the diff.
	m.cursor = 0
	wheel(x, tea.MouseWheelDown)
	if m.cursor != wheelStep {
		t.Fatalf("wheel down: cursor=%d", m.cursor)
	}
	wheel(x, tea.MouseWheelUp)
	if m.cursor != 0 {
		t.Fatalf("wheel up: cursor=%d", m.cursor)
	}
	// File panel: flat list click opens the file and focuses the panel.
	m.tree = false
	m.fileScroll = 0
	click(1, headerH+paneHeaderH+1, 0)
	if m.fileIdx != 1 || !m.filesFocused {
		t.Fatalf("flat click: fileIdx=%d focused=%v", m.fileIdx, m.filesFocused)
	}
	click(1, headerH+paneHeaderH+7, 0) // past the end: ignored
	if m.fileIdx != 1 {
		t.Fatal("click past the list should be ignored")
	}
	// Tree: clicking a file node opens it; the wheel walks the list.
	m.tree = true
	m.rebuildTree()
	var mainIdx int
	for i, n := range m.treeNodes {
		if !n.isDir && m.files[n.fileIdx].Path() == "main.go" {
			mainIdx = i
		}
	}
	click(1, headerH+paneHeaderH+mainIdx, 0)
	if m.fileIdx != 0 || m.treeSel != "main.go" {
		t.Fatalf("tree click: fileIdx=%d sel=%q", m.fileIdx, m.treeSel)
	}
	wheel(1, tea.MouseWheelDown)
	if m.fileIdx == 0 {
		wheel(1, tea.MouseWheelUp) // main.go sorts last: the neighbour is above
	}
	if m.fileIdx != 1 {
		t.Fatalf("wheel in the file panel: fileIdx=%d", m.fileIdx)
	}
	// Overlays swallow the mouse; a click closes the help.
	m.overlay = overlayHelp
	click(x, rowY(3), 0)
	if m.overlay != overlayNone {
		t.Fatal("click should close the help")
	}
	m.overlay = overlayReview
	m.cursor = 0
	click(x, rowY(3), 0)
	if m.cursor != 0 || m.overlay != overlayReview {
		t.Fatal("clicks under an overlay should be ignored")
	}
}
