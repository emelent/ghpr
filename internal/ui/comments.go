package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
)

// ---------- comments screen (i) ----------

// cmMaxSnippet caps how many code lines a multi-line thread shows.
const cmMaxSnippet = 8

// snippetLine is one diff line a thread is anchored to.
type snippetLine struct {
	sign string
	kind diff.Kind
	text string
}

// openComments switches to the list of every review thread in the PR.
func (m *Model) openComments() tea.Cmd {
	if m.pr == nil {
		return nil
	}
	m.buildCommentList()
	if len(m.cmThreads) == 0 {
		return m.setStatus("No review threads on this PR", true)
	}
	m.screen = screenComments
	m.cmScroll = 0
	// Preselect the thread under the cursor, if any.
	if r := m.currentRow(); r != nil && r.kind == rowThread {
		for i, t := range m.cmThreads {
			if t.ID == r.thread.ID {
				m.cmIdx = i
				break
			}
		}
	}
	if m.cmIdx >= len(m.cmThreads) {
		m.cmIdx = 0
	}
	return nil
}

// buildCommentList orders the threads by file (in diff order) and line.
func (m *Model) buildCommentList() {
	fileOrder := map[string]int{}
	for i := range m.files {
		fileOrder[m.files[i].Path()] = i
	}
	m.cmThreads = m.cmThreads[:0]
	for i := range m.threads {
		m.cmThreads = append(m.cmThreads, &m.threads[i])
	}
	pos := func(t *gh.Thread) (int, int) {
		fi, ok := fileOrder[t.Path]
		if !ok {
			fi = len(m.files)
		}
		return fi, max(t.Line, t.OriginalLine)
	}
	sort.SliceStable(m.cmThreads, func(a, b int) bool {
		fa, la := pos(m.cmThreads[a])
		fb, lb := pos(m.cmThreads[b])
		if fa != fb {
			return fa < fb
		}
		return la < lb
	})
	if m.cmIdx >= len(m.cmThreads) {
		m.cmIdx = max(0, len(m.cmThreads)-1)
	}
}

// closeComments returns to the diff.
func (m *Model) closeComments() {
	m.screen = screenDiff
	m.backTo = screenDiff
}

// handleCommentsKey drives the comments screen.
func (m *Model) handleCommentsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "i", "q":
		m.closeComments()
	case "j", "down":
		if m.cmIdx+1 < len(m.cmThreads) {
			m.cmIdx++
		}
	case "k", "up":
		if m.cmIdx > 0 {
			m.cmIdx--
		}
	case "g", "home":
		m.cmIdx = 0
	case "G", "end":
		m.cmIdx = max(0, len(m.cmThreads)-1)
	case "l", "right", "enter":
		return m, m.openCommentThread()
	case "R":
		return m, m.loadAll()
	case "?":
		m.screen = screenDiff
		m.overlay = overlayHelp
	case "Q", "ctrl+c":
		m.savePosition()
		return m, tea.Quit
	}
	return m, nil
}

// openCommentThread shows the selected thread in the diff; h there comes
// back to this list.
func (m *Model) openCommentThread() tea.Cmd {
	if m.cmIdx >= len(m.cmThreads) {
		return nil
	}
	t := m.cmThreads[m.cmIdx]
	fi := -1
	for i := range m.files {
		if m.files[i].Path() == t.Path {
			fi = i
			break
		}
	}
	if fi < 0 {
		return m.setStatus("Thread's file is not in the diff: "+t.Path, true)
	}
	m.screen = screenDiff
	m.backTo = screenComments
	m.filesFocused = false
	cmd := m.selectFile(fi)
	for i := range m.rows {
		if m.rows[i].kind == rowThread && m.rows[i].thread.ID == t.ID {
			m.cursor = i
			break
		}
	}
	return cmd
}

// threadSnippet returns the diff lines a thread is anchored to, every line
// of a multi-line comment's range (StartLine..Line on the thread's side),
// or nil when the thread no longer maps onto the diff. hidden is how many
// more lines the range has beyond cmMaxSnippet.
func (m *Model) threadSnippet(t *gh.Thread) (lines []snippetLine, hidden int) {
	if t.Line == 0 {
		return nil, 0
	}
	lo, hi := t.Line, t.Line
	if t.StartLine > 0 && t.StartLine < t.Line {
		lo = t.StartLine
	}
	for fi := range m.files {
		f := &m.files[fi]
		if f.Path() != t.Path {
			continue
		}
		for hi2 := range f.Hunks {
			for li := range f.Hunks[hi2].Lines {
				l := &f.Hunks[hi2].Lines[li]
				var n int
				switch t.DiffSide {
				case "LEFT":
					if l.Kind == diff.Add {
						continue
					}
					n = l.OldNum
				default:
					if l.Kind == diff.Del {
						continue
					}
					n = l.NewNum
				}
				if n < lo || n > hi {
					continue
				}
				if len(lines) == cmMaxSnippet {
					hidden++
					continue
				}
				s, _ := signOf(l.Kind)
				lines = append(lines, snippetLine{sign: s, kind: l.Kind, text: expandTabs(l.Text)})
			}
		}
		break
	}
	return lines, hidden
}

// commentItemHeight is the number of lines a thread takes: location, its
// code lines (or one note), a "more" line when capped, first comment, spacer.
func (m *Model) commentItemHeight(t *gh.Thread) int {
	lines, hidden := m.threadSnippet(t)
	h := 3 + max(1, len(lines))
	if hidden > 0 {
		h++
	}
	return h
}

// renderComments draws the comments screen body.
func (m *Model) renderComments(width, height int) []string {
	open := 0
	for _, t := range m.cmThreads {
		if !t.IsResolved {
			open++
		}
	}
	title := styTitle.Render(fmt.Sprintf(" Comments (%d threads · %d open)", len(m.cmThreads), open))
	lines := []string{padRight(truncate(title, width), width), styBorder.Render(strings.Repeat("─", width))}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	// Keep the selected thread fully visible (items vary in height).
	if m.cmIdx < m.cmScroll {
		m.cmScroll = m.cmIdx
	}
	for m.cmScroll < m.cmIdx {
		used := 0
		for i := m.cmScroll; i <= m.cmIdx; i++ {
			used += m.commentItemHeight(m.cmThreads[i])
		}
		if used <= avail {
			break
		}
		m.cmScroll++
	}
	for i := m.cmScroll; i < len(m.cmThreads) && len(lines) < height; i++ {
		lines = append(lines, m.renderCommentItem(m.cmThreads[i], i == m.cmIdx, width)...)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}

// renderCommentItem renders one thread as commentItemHeight lines.
func (m *Model) renderCommentItem(t *gh.Thread, selected bool, width int) []string {
	bg := color.Color(lipgloss.NoColor{})
	if selected {
		bg = colSelBg
	}
	base := lipgloss.NewStyle().Background(bg)
	marker := base.Foreground(colWarn).Render("● ")
	status := base.Foreground(colWarn).Render("open")
	if t.IsResolved {
		marker = base.Foreground(colOK).Render("✓ ")
		status = base.Foreground(colOK).Render("resolved")
	}
	if t.IsOutdated {
		status += base.Foreground(colDim).Render(" · outdated")
	}
	loc := t.Path
	switch {
	case t.Line > 0 && t.StartLine > 0 && t.StartLine != t.Line:
		loc += fmt.Sprintf(":%d-%d", t.StartLine, t.Line)
	case t.Line > 0:
		loc += fmt.Sprintf(":%d", t.Line)
	case t.OriginalLine > 0:
		loc += fmt.Sprintf(":%d (orig)", t.OriginalLine)
	}
	meta := ""
	if len(t.Comments) > 0 {
		c := t.Comments[0]
		meta = fmt.Sprintf("  @%s · %s · %d comment(s) · ", c.Author, ago(c.CreatedAt), len(t.Comments))
	}
	l1 := " " + marker + base.Foreground(colText).Bold(true).Render(loc) + base.Foreground(colDim).Render(meta) + status

	out := []string{padRightBg(truncate(l1, width), width, bg)}
	snippet, hidden := m.threadSnippet(t)
	if len(snippet) == 0 {
		note := base.Render("     ") + base.Foreground(colDim).Italic(true).Render("(line no longer in the diff)")
		out = append(out, padRightBg(truncateTail(note, width), width, bg))
	}
	for _, sl := range snippet {
		lineBgCol := lineBg(sl.kind, stNormal)
		if selected {
			lineBgCol = bg
		}
		_, signFg := signOf(sl.kind)
		code := lipgloss.NewStyle().Background(lineBgCol)
		l := base.Render("     ") + code.Foreground(signFg).Bold(true).Render(sl.sign+" ") +
			code.Foreground(colText).Render(strings.TrimRight(sl.text, " "))
		out = append(out, padRightBg(truncateTail(l, width), width, bg))
	}
	if hidden > 0 {
		more := base.Render("     ") + base.Foreground(colDim).Italic(true).Render(fmt.Sprintf("… %d more line(s)", hidden))
		out = append(out, padRightBg(truncateTail(more, width), width, bg))
	}

	body := ""
	if len(t.Comments) > 0 {
		body = strings.TrimSpace(strings.ReplaceAll(t.Comments[0].Body, "\r", ""))
		body, _, _ = strings.Cut(body, "\n")
	}
	l3 := base.Render("     ") + base.Foreground(colText).Render(body)
	return append(out, padRightBg(truncateTail(l3, width), width, bg), padRightBg("", width, bg))
}

// commentAtY maps a screen row to a comments-screen item index, or -1.
func (m *Model) commentAtY(y int) int {
	top := headerH + paneHeaderH
	if y < top {
		return -1
	}
	off := y - top
	for i := m.cmScroll; i < len(m.cmThreads); i++ {
		h := m.commentItemHeight(m.cmThreads[i])
		if off < h {
			return i
		}
		off -= h
	}
	return -1
}
