package ui

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"ghpr/internal/diff"
)

// ---------- fuzzy file picker (ctrl+p) ----------

// fpMatch is one file offered by the picker.
type fpMatch struct {
	fileIdx int
	matched []int // byte offsets into the path that matched the query
}

// filePaths lets the fuzzy matcher read the paths of the shown files.
type filePaths struct {
	files []diff.File
	idx   []int // indexes into files that are offered
}

func (p filePaths) String(i int) string { return p.files[p.idx[i]].Path() }
func (p filePaths) Len() int            { return len(p.idx) }

// openFilePicker opens the fuzzy file finder over the diff's files.
func (m *Model) openFilePicker() {
	if len(m.files) == 0 {
		return
	}
	m.overlay = overlayFiles
	m.fpQuery = ""
	m.fpIdx, m.fpScroll = 0, 0
	m.refilterFiles()
}

// refilterFiles recomputes the picker's results for the current query.
// Spaces in the query are ignored so "cmd main" finds cmd/x/main.go.
func (m *Model) refilterFiles() {
	m.fpResults = m.fpResults[:0]
	q := strings.ReplaceAll(m.fpQuery, " ", "")
	shown := m.shownFiles()
	if q == "" {
		for _, i := range shown {
			m.fpResults = append(m.fpResults, fpMatch{fileIdx: i})
		}
	} else {
		for _, r := range fuzzy.FindFrom(q, filePaths{files: m.files, idx: shown}) {
			m.fpResults = append(m.fpResults, fpMatch{fileIdx: shown[r.Index], matched: r.MatchedIndexes})
		}
	}
	if m.fpIdx >= len(m.fpResults) {
		m.fpIdx = max(0, len(m.fpResults)-1)
	}
}

// handleFilePickerKey edits the query and moves through the results.
func (m *Model) handleFilePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.overlay = overlayNone
		return m, nil
	case "enter":
		if len(m.fpResults) == 0 {
			return m, nil
		}
		idx := m.fpResults[m.fpIdx].fileIdx
		m.overlay = overlayNone
		m.filesFocused = false
		return m, m.selectFile(idx)
	case "down", "ctrl+n", "ctrl+j", "tab":
		if m.fpIdx+1 < len(m.fpResults) {
			m.fpIdx++
		}
		return m, nil
	case "up", "ctrl+k", "shift+tab":
		if m.fpIdx > 0 {
			m.fpIdx--
		}
		return m, nil
	case "backspace":
		if r := []rune(m.fpQuery); len(r) > 0 {
			m.fpQuery = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.fpQuery = ""
	default:
		if msg.Text == "" {
			return m, nil // control key: ignore
		}
		m.fpQuery += msg.Text
	}
	m.fpIdx, m.fpScroll = 0, 0
	m.refilterFiles()
	return m, nil
}

// renderFilePicker draws the picker in place of the diff pane.
func (m *Model) renderFilePicker(width, height int) []string {
	title := styTitle.Render(" Go to file") + styDim.Render(fmt.Sprintf("  %d of %d", len(m.fpResults), len(m.shownFiles())))
	lines := []string{
		padRight(truncate(title, width), width),
		styBorder.Render(strings.Repeat("─", width)),
		padRight(truncate(styAccent.Render(" > ")+m.fpQuery+styAccent.Render("▏"), width), width),
	}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	if len(m.fpResults) == 0 {
		lines = append(lines, padRight(styNote.Render("   No file matches"), width))
	}
	// Keep the selection visible.
	if m.fpIdx < m.fpScroll {
		m.fpScroll = m.fpIdx
	}
	if m.fpIdx >= m.fpScroll+avail {
		m.fpScroll = m.fpIdx - avail + 1
	}
	for i := m.fpScroll; i < len(m.fpResults) && len(lines) < height; i++ {
		lines = append(lines, m.renderPickerRow(&m.fpResults[i], i == m.fpIdx, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// renderPickerRow renders "✓M path +a -d" with the matched characters of the
// path emphasised; the selected row is drawn on the selection background.
func (m *Model) renderPickerRow(r *fpMatch, selected bool, width int) string {
	f := &m.files[r.fileIdx]
	bg := color.Color(lipgloss.NoColor{})
	if selected {
		bg = colSelBg
	}
	base := lipgloss.NewStyle().Background(bg)
	hit := base.Foreground(colAccent).Bold(true).Underline(true)
	plain := base.Foreground(colText)
	if !selected && m.isViewed(f.Path()) {
		plain = base.Foreground(colDim)
	}
	mark := base.Render(" ")
	if m.isViewed(f.Path()) {
		mark = base.Foreground(colOK).Render("✓")
	}
	counts := base.Foreground(colOK).Render(fmt.Sprintf("+%d", f.Additions)) + base.Render(" ") +
		base.Foreground(colErr).Render(fmt.Sprintf("-%d", f.Deletions)) + base.Render(" ")

	// Path with fuzzy hits, walked by byte offset to match the matcher.
	matched := map[int]bool{}
	for _, b := range r.matched {
		matched[b] = true
	}
	var sb strings.Builder
	path := f.Path()
	for off := 0; off < len(path); {
		_, size := utf8.DecodeRuneInString(path[off:])
		ch := path[off : off+size]
		if matched[off] {
			sb.WriteString(hit.Render(ch))
		} else {
			sb.WriteString(plain.Render(ch))
		}
		off += size
	}
	head := mark + base.Render(ansi.Strip(statusLetter(f.Status))+" ")
	pathW := width - ansi.StringWidth(head) - ansi.StringWidth(counts)
	if pathW < 4 {
		counts = ""
		pathW = max(1, width-ansi.StringWidth(head))
	}
	line := head + padRightBg(truncate(sb.String(), pathW), pathW, bg) + counts
	return padRightBg(truncate(line, width), width, bg)
}
