package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------- mouse ----------

// wheelStep is how many rows a wheel notch moves the cursor.
const wheelStep = 3

// headerH is the number of lines above the main body (PR header).
const headerH = 2

// paneHeaderH is the number of lines above the rows in the diff pane and
// the file panel (title + border).
const paneHeaderH = 2

// handleMouse dispatches click, drag, release and wheel events. Only the
// diff screen with no overlay reacts, except that any click closes the help,
// and in the PR list the wheel moves and a click selects (a second click on
// the selected PR opens it).
func (m *Model) handleMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.screen == screenPicker {
		switch e := msg.(type) {
		case tea.MouseWheelMsg:
			switch e.Button {
			case tea.MouseWheelUp:
				m.list.CursorUp()
			case tea.MouseWheelDown:
				m.list.CursorDown()
			}
		case tea.MouseClickMsg:
			if e.Button == tea.MouseLeft {
				return m, m.clickList(e.Y)
			}
		}
		return m, nil
	}
	if m.overlay == overlayHelp {
		if _, ok := msg.(tea.MouseClickMsg); ok {
			m.overlay = overlayNone
		}
		return m, nil
	}
	if m.overlay != overlayNone {
		return m, nil
	}
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		return m, m.mouseWheel(e)
	case tea.MouseClickMsg:
		if e.Button != tea.MouseLeft {
			return m, nil
		}
		return m, m.mouseClick(e)
	case tea.MouseMotionMsg:
		if m.dragging && e.Button == tea.MouseLeft {
			m.mouseDrag(e)
		}
	case tea.MouseReleaseMsg:
		if e.Button == tea.MouseLeft {
			m.mouseRelease()
		}
	}
	return m, nil
}

// inFilesPanel reports whether x falls inside the file panel.
func (m *Model) inFilesPanel(x int) bool { return m.showFiles && x < m.filesWidth() }

// inDiffPane reports whether x falls inside the diff pane (after the
// separator when the file panel is shown).
func (m *Model) inDiffPane(x int) bool {
	if !m.showFiles {
		return x >= 0 && x < m.width
	}
	return x > m.filesWidth() && x < m.width
}

// rowAtY maps a screen row to a diff row index. clamp makes positions above
// or below the rows snap to the nearest visible row (used while dragging);
// otherwise those return -1.
func (m *Model) rowAtY(y int, clamp bool) int {
	if len(m.rows) == 0 {
		return -1
	}
	top := headerH + paneHeaderH
	bottom := headerH + paneHeaderH + m.viewportH() // exclusive
	switch {
	case y < top:
		if !clamp {
			return -1
		}
		y = top
	case y >= bottom:
		if !clamp {
			return -1
		}
		y = bottom - 1
	}
	off := m.scroll + (y - top)
	for i := range m.rows {
		if off < m.rowStart[i]+m.rowHeight(i) {
			return i
		}
	}
	if clamp {
		return len(m.rows) - 1
	}
	return -1
}

func (m *Model) mouseWheel(e tea.MouseWheelMsg) tea.Cmd {
	dir := 0
	switch e.Button {
	case tea.MouseWheelUp:
		dir = -1
	case tea.MouseWheelDown:
		dir = 1
	default:
		return nil
	}
	if m.inFilesPanel(e.X) {
		key := "j"
		if dir < 0 {
			key = "k"
		}
		_, cmd := m.handleFilesKey(key)
		return cmd
	}
	if m.inDiffPane(e.X) {
		m.moveCursor(dir * wheelStep)
	}
	return nil
}

// mouseClick moves the cursor to the clicked diff row (shift extends the
// selection) or picks a file / toggles a directory in the file panel.
func (m *Model) mouseClick(e tea.MouseClickMsg) tea.Cmd {
	if m.inFilesPanel(e.X) {
		return m.clickFiles(e.Y)
	}
	if !m.inDiffPane(e.X) {
		return nil
	}
	i := m.rowAtY(e.Y, false)
	if i < 0 {
		return nil
	}
	m.filesFocused = false
	if e.Mod&tea.ModShift != 0 {
		if !m.selecting {
			m.selecting = true
			m.selAnchor = m.cursor
		}
		m.cursor = i
		return nil
	}
	m.clearSelection()
	m.cursor = i
	m.dragging = true
	m.dragAnchor = i
	return nil
}

// mouseDrag extends a selection from the row where the button went down to
// the row under the pointer.
func (m *Model) mouseDrag(e tea.MouseMotionMsg) {
	i := m.rowAtY(e.Y, true)
	if i < 0 {
		return
	}
	if i != m.dragAnchor && !m.selecting {
		m.selecting = true
		m.selAnchor = m.dragAnchor
	}
	m.cursor = i
}

// mouseRelease ends a drag; a drag that came back to its origin leaves no
// selection behind.
func (m *Model) mouseRelease() {
	m.dragging = false
	if m.selecting && m.selAnchor == m.cursor {
		m.clearSelection()
	}
}

// listTopHeight is the number of lines above the first item in the PR list
// (title bar and status bar, as the bubbles list lays them out).
func (m *Model) listTopHeight() int {
	h := lipgloss.Height(m.list.Styles.TitleBar.Render(m.list.Styles.Title.Render(m.list.Title)))
	if m.list.ShowStatusBar() {
		h += lipgloss.Height(m.list.Styles.StatusBar.Render(" "))
	}
	return h
}

// clickList selects the PR under screen row y; clicking the PR that is
// already selected opens it.
func (m *Model) clickList(y int) tea.Cmd {
	slot := m.listItemH + m.listItemGap
	top := m.listTopHeight()
	if slot <= 0 || y < top {
		return nil
	}
	rel := y - top
	if rel%slot >= m.listItemH {
		return nil // the blank line between items
	}
	items := m.list.VisibleItems()
	start, end := m.list.Paginator.GetSliceBounds(len(items))
	idx := start + rel/slot
	if idx >= end {
		return nil
	}
	if idx == m.list.Index() {
		return m.openSelectedPR()
	}
	m.list.Select(idx)
	return nil
}

// clickFiles handles a click at screen row y inside the file panel.
func (m *Model) clickFiles(y int) tea.Cmd {
	idx := m.fileScroll + (y - headerH - paneHeaderH)
	if y < headerH+paneHeaderH || idx < 0 {
		return nil
	}
	m.filesFocused = true
	if !m.tree {
		shown := m.shownFiles()
		if idx >= len(shown) {
			return nil
		}
		return m.selectFile(shown[idx])
	}
	if len(m.treeNodes) == 0 {
		m.rebuildTree()
	}
	if idx >= len(m.treeNodes) {
		return nil
	}
	n := &m.treeNodes[idx]
	m.treeSel = n.path
	if n.isDir {
		m.collapsed[n.path] = !m.collapsed[n.path]
		m.rebuildTree()
		return nil
	}
	return m.selectFile(n.fileIdx)
}
