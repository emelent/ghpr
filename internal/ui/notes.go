package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ghpr/internal/diff"
	"ghpr/internal/state"
)

// ---------- notes on lines (a / A) ----------

// lineNote is a note the reviewer left on a line to come back to. Notes
// are kept in the state store per PR, so they are there again on re-open;
// without a store they last for the session.
type lineNote struct {
	path           string
	oldNum, newNum int // line identity; newNum > 0 means the new side
	kind           diff.Kind
	text           string // code on the line, for the list
	body           string // what the reviewer wanted to do here
	createdAt      time.Time
}

var kindNames = map[diff.Kind]string{diff.Context: "context", diff.Add: "add", diff.Del: "del"}

// toState converts a note for the store.
func (nt *lineNote) toState() state.Note {
	return state.Note{Path: nt.path, OldLine: nt.oldNum, NewLine: nt.newNum, Kind: kindNames[nt.kind], Text: nt.text, Body: nt.body, CreatedAt: nt.createdAt}
}

// noteFromState converts a stored note back.
func noteFromState(n state.Note) lineNote {
	kind := diff.Context
	for k, name := range kindNames {
		if name == n.Kind {
			kind = k
		}
	}
	return lineNote{path: n.Path, oldNum: n.OldLine, newNum: n.NewLine, kind: kind, text: n.Text, body: n.Body, createdAt: n.CreatedAt}
}

// loadNotes restores the PR's notes from the store.
func (m *Model) loadNotes() {
	if m.store == nil {
		return
	}
	m.notes = m.notes[:0]
	for _, n := range m.store.GetNotes(m.prKey()) {
		m.notes = append(m.notes, noteFromState(n))
	}
	if m.ntIdx >= len(m.notes) {
		m.ntIdx = 0
	}
}

// saveNotes writes the PR's notes to the store.
func (m *Model) saveNotes() tea.Cmd {
	if m.store == nil {
		return nil
	}
	sn := make([]state.Note, 0, len(m.notes))
	for i := range m.notes {
		sn = append(sn, m.notes[i].toState())
	}
	m.store.SetNotes(m.prKey(), sn)
	if err := m.store.Save(); err != nil {
		return m.setStatus("Save notes: "+err.Error(), true)
	}
	return nil
}

// ntItemH is the number of lines one note takes in the notes screen:
// location + note, code line, spacer.
const ntItemH = 3

// matchesLine reports whether the note is on diff line l of path.
func (nt *lineNote) matchesLine(path string, l *diff.Line) bool {
	if l == nil || nt.path != path {
		return false
	}
	if nt.newNum > 0 {
		return l.Kind != diff.Del && l.NewNum == nt.newNum
	}
	return l.Kind != diff.Add && l.OldNum == nt.oldNum
}

// noteIndexForLine returns the index of the note on line l of the current
// file, or -1.
func (m *Model) noteIndexForLine(l *diff.Line) int {
	if l == nil || len(m.files) == 0 {
		return -1
	}
	path := m.files[m.fileIdx].Path()
	for i := range m.notes {
		if m.notes[i].matchesLine(path, l) {
			return i
		}
	}
	return -1
}

// rowNoteIndex returns the index of the note on row i, or -1.
func (m *Model) rowNoteIndex(i int) int {
	if i < 0 || i >= len(m.rows) {
		return -1
	}
	r := &m.rows[i]
	switch r.kind {
	case rowLine:
		return m.noteIndexForLine(r.line)
	case rowSplit:
		if k := m.noteIndexForLine(r.right); k >= 0 {
			return k
		}
		return m.noteIndexForLine(r.left)
	}
	return -1
}

// rowNoteLine picks the line a new note on row i attaches to (the new side
// when there is one), or nil when the row is not a diff line.
func (m *Model) rowNoteLine(i int) *diff.Line {
	r := &m.rows[i]
	switch r.kind {
	case rowLine:
		return r.line
	case rowSplit:
		if r.right != nil {
			return r.right
		}
		return r.left
	}
	return nil
}

// toggleNote (a) removes the note on the cursor line, or asks for the text
// of a new one.
func (m *Model) toggleNote() tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	if k := m.rowNoteIndex(m.cursor); k >= 0 {
		nt := m.notes[k]
		m.notes = append(m.notes[:k], m.notes[k+1:]...)
		return tea.Batch(m.saveNotes(), m.setStatus(fmt.Sprintf("Note removed: %s — %s", nt.location(), nt.body), false))
	}
	l := m.rowNoteLine(m.cursor)
	if l == nil {
		return m.setStatus("Move the cursor onto a diff line to add a note", true)
	}
	m.ntPending = lineNote{path: m.files[m.fileIdx].Path(), oldNum: l.OldNum, newNum: l.NewNum, kind: l.Kind, text: expandTabs(l.Text)}
	if l.Kind == diff.Del {
		m.ntPending.newNum = 0
	}
	m.ntInput = ""
	m.overlay = overlayNote
	return nil
}

// handleNoteKey edits the text prompt for a new note.
func (m *Model) handleNoteKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.overlay = overlayNone
		return m, nil
	case "enter":
		body := strings.TrimSpace(m.ntInput)
		if body == "" {
			return m, m.setStatus("Type the note first so you remember what to do there", true)
		}
		m.overlay = overlayNone
		m.ntPending.body = body
		m.ntPending.createdAt = time.Now()
		m.notes = append(m.notes, m.ntPending)
		return m, tea.Batch(m.saveNotes(), m.setStatus(fmt.Sprintf("Note added at %s — %s (%d note(s), A lists them)", m.ntPending.location(), body, len(m.notes)), false))
	case "backspace":
		if r := []rune(m.ntInput); len(r) > 0 {
			m.ntInput = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.ntInput = ""
	default:
		if msg.Text == "" {
			return m, nil
		}
		m.ntInput += msg.Text
	}
	return m, nil
}

// location formats "path:line" with a minus for old-side lines.
func (nt *lineNote) location() string {
	if nt.newNum > 0 {
		return fmt.Sprintf("%s:%d", nt.path, nt.newNum)
	}
	return fmt.Sprintf("%s:-%d", nt.path, nt.oldNum)
}

// openNotes (A) shows the list of notes.
func (m *Model) openNotes() tea.Cmd {
	if len(m.notes) == 0 {
		return m.setStatus("No notes yet – press a on a line to add one", true)
	}
	m.screen = screenNotes
	m.ntScroll = 0
	if k := m.rowNoteIndex(m.cursor); k >= 0 {
		m.ntIdx = k
	}
	if m.ntIdx >= len(m.notes) {
		m.ntIdx = 0
	}
	return nil
}

// handleNotesKey drives the notes screen.
func (m *Model) handleNotesKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "A", "q":
		m.screen = screenDiff
		m.backTo = screenDiff
	case "j", "down":
		if m.ntIdx+1 < len(m.notes) {
			m.ntIdx++
		}
	case "k", "up":
		if m.ntIdx > 0 {
			m.ntIdx--
		}
	case "g", "home":
		m.ntIdx = 0
	case "G", "end":
		m.ntIdx = max(0, len(m.notes)-1)
	case "l", "right", "enter":
		return m, m.openNote()
	case "d", "x":
		if m.ntIdx < len(m.notes) {
			m.notes = append(m.notes[:m.ntIdx], m.notes[m.ntIdx+1:]...)
			if m.ntIdx >= len(m.notes) {
				m.ntIdx = max(0, len(m.notes)-1)
			}
			save := m.saveNotes()
			if len(m.notes) == 0 {
				m.screen = screenDiff
				m.backTo = screenDiff
				return m, tea.Batch(save, m.setStatus("Last note removed", false))
			}
			return m, save
		}
	case "?":
		m.screen = screenDiff
		m.overlay = overlayHelp
	case "Q", "ctrl+c":
		m.savePosition()
		return m, tea.Quit
	}
	return m, nil
}

// openNote jumps to the selected note in the diff; h there comes back.
func (m *Model) openNote() tea.Cmd {
	if m.ntIdx >= len(m.notes) {
		return nil
	}
	nt := m.notes[m.ntIdx]
	fi := -1
	for i := range m.files {
		if m.files[i].Path() == nt.path {
			fi = i
			break
		}
	}
	if fi < 0 {
		return m.setStatus("Noted file is no longer in the diff: "+nt.path, true)
	}
	m.screen = screenDiff
	m.backTo = screenNotes
	m.filesFocused = false
	cmd := m.selectFile(fi)
	for i := range m.rows {
		if m.rowNoteIndex(i) == m.ntIdx {
			m.cursor = i
			break
		}
	}
	return cmd
}

// renderNotes draws the notes screen body.
func (m *Model) renderNotes(width, height int) []string {
	title := styTitle.Render(fmt.Sprintf(" Notes (%d)", len(m.notes))) + styDim.Render("  lines to come back to · a adds one in the diff")
	lines := []string{padRight(truncate(title, width), width), styBorder.Render(strings.Repeat("─", width))}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	perPage := max(1, avail/ntItemH)
	if m.ntIdx < m.ntScroll {
		m.ntScroll = m.ntIdx
	}
	if m.ntIdx >= m.ntScroll+perPage {
		m.ntScroll = m.ntIdx - perPage + 1
	}
	for i := m.ntScroll; i < len(m.notes) && len(lines)+ntItemH <= height; i++ {
		lines = append(lines, m.renderNoteItem(&m.notes[i], i == m.ntIdx, width)...)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}

// renderNoteItem renders one note as ntItemH lines.
func (m *Model) renderNoteItem(nt *lineNote, selected bool, width int) []string {
	bg := color.Color(lipgloss.NoColor{})
	if selected {
		bg = colSelBg
	}
	base := lipgloss.NewStyle().Background(bg)
	l1 := " " + base.Foreground(colWarn).Render("⚑ ") + base.Foreground(colText).Bold(true).Render(nt.location()) +
		base.Foreground(colDim).Render("  ") + base.Foreground(colAccent).Render(nt.body)
	lineBgCol := lineBg(nt.kind, stNormal)
	if selected {
		lineBgCol = bg
	}
	sign, signFg := signOf(nt.kind)
	code := lipgloss.NewStyle().Background(lineBgCol)
	l2 := base.Render("     ") + code.Foreground(signFg).Bold(true).Render(sign+" ") + code.Foreground(colText).Render(strings.TrimRight(nt.text, " "))
	return []string{
		padRightBg(truncate(l1, width), width, bg),
		padRightBg(truncateTail(l2, width), width, bg),
		padRightBg("", width, bg),
	}
}

// noteAtY maps a screen row to a notes-screen item index, or -1.
func (m *Model) noteAtY(y int) int {
	top := headerH + paneHeaderH
	if y < top {
		return -1
	}
	i := m.ntScroll + (y-top)/ntItemH
	if i >= len(m.notes) {
		return -1
	}
	return i
}
