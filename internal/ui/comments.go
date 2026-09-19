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
	"ghpr/internal/keys"
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
	if len(m.threads) == 0 {
		return m.setStatus("No review threads on this PR", true)
	}
	m.buildCommentList()
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

// buildCommentList orders the threads by file (in diff order) and line,
// leaving out resolved threads unless cmShowResolved is set and threads
// that do not match the active search. The selection follows its thread
// when the list changes and clamps when it is gone.
func (m *Model) buildCommentList() {
	fileOrder := map[string]int{}
	for i := range m.files {
		fileOrder[m.files[i].Path()] = i
	}
	selID := ""
	if m.cmIdx < len(m.cmThreads) {
		selID = m.cmThreads[m.cmIdx].ID
	}
	m.cmThreads = m.cmThreads[:0]
	for i := range m.threads {
		if m.threads[i].IsResolved && !m.cmShowResolved {
			continue
		}
		if !threadMatches(&m.threads[i], m.cmQuery) {
			continue
		}
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
	for i, t := range m.cmThreads {
		if t.ID == selID {
			m.cmIdx = i
			break
		}
	}
	if m.cmIdx >= len(m.cmThreads) {
		m.cmIdx = max(0, len(m.cmThreads)-1)
	}
}

// threadMatches reports whether a thread is a hit for the comments-screen
// search: the query occurs in its file path, in the name of anyone who
// wrote in it, or in the text of any of its comments. An empty query
// matches everything.
func threadMatches(t *gh.Thread, q string) bool {
	if q == "" {
		return true
	}
	if len(findMatches(t.Path, q)) > 0 {
		return true
	}
	for i := range t.Comments {
		if len(findMatches(t.Comments[i].Author, q)) > 0 || len(findMatches(t.Comments[i].Body, q)) > 0 {
			return true
		}
	}
	return false
}

// cmCandidates is how many threads the resolved filter lets through, which
// is what the search then narrows.
func (m *Model) cmCandidates() int {
	n := 0
	for i := range m.threads {
		if !m.threads[i].IsResolved || m.cmShowResolved {
			n++
		}
	}
	return n
}

// firstLine is the first line of a comment body, trimmed.
func firstLine(body string) string {
	b := strings.TrimSpace(strings.ReplaceAll(body, "\r", ""))
	b, _, _ = strings.Cut(b, "\n")
	return b
}

// cmPreview picks the comment of a thread to show in the list and the line
// of its body to show with it. With a search active that is the first
// comment and the first line of it that the query hits, so the list shows
// what was matched; otherwise the first comment and its opening line.
func cmPreview(t *gh.Thread, q string) (c *gh.Comment, line string) {
	if len(t.Comments) == 0 {
		return nil, ""
	}
	if q != "" {
		for i := range t.Comments {
			for _, l := range strings.Split(strings.ReplaceAll(t.Comments[i].Body, "\r", ""), "\n") {
				if len(findMatches(l, q)) > 0 {
					return &t.Comments[i], strings.TrimSpace(l)
				}
			}
			if len(findMatches(t.Comments[i].Author, q)) > 0 {
				return &t.Comments[i], firstLine(t.Comments[i].Body)
			}
		}
	}
	return &t.Comments[0], firstLine(t.Comments[0].Body)
}

// resolvedHidden is how many resolved threads the list leaves out.
func (m *Model) resolvedHidden() int {
	if m.cmShowResolved {
		return 0
	}
	n := 0
	for i := range m.threads {
		if m.threads[i].IsResolved {
			n++
		}
	}
	return n
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
		// An active filter goes first, so the list is whole again before
		// the key closes the screen.
		if m.cmQuery != "" {
			m.cmQuery = ""
			m.buildCommentList()
			return m, nil
		}
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
	case "x":
		if m.busy != "" || m.cmIdx >= len(m.cmThreads) {
			return m, nil
		}
		return m, m.resolveThread(m.cmThreads[m.cmIdx])
	case "r":
		// Reply without leaving the list: the input panel opens under it.
		if m.busy != "" || m.cmIdx >= len(m.cmThreads) {
			return m, nil
		}
		return m, m.replyTo(m.cmThreads[m.cmIdx])
	case "t":
		m.cmShowResolved = !m.cmShowResolved
		m.buildCommentList()
	case "/":
		m.cmSearching = true
		m.cmSearchPrev = m.cmQuery
		m.cmSearchInput = m.cmQuery
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

// handleCommentsSearchKey edits the / prompt. Typing filters the list as
// you go, enter keeps the filter, esc puts back whatever was in force
// before the prompt was opened.
func (m *Model) handleCommentsSearchKey(key string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.cmSearching = false
		m.cmQuery = m.cmSearchPrev
		m.buildCommentList()
		return m, nil
	case "enter":
		m.cmSearching = false
		m.cmQuery = m.cmSearchInput
		m.buildCommentList()
		return m, nil
	case "down":
		if m.cmIdx+1 < len(m.cmThreads) {
			m.cmIdx++
		}
		return m, nil
	case "up":
		if m.cmIdx > 0 {
			m.cmIdx--
		}
		return m, nil
	case "backspace":
		if r := []rune(m.cmSearchInput); len(r) > 0 {
			m.cmSearchInput = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.cmSearchInput = ""
	default:
		if msg.Text == "" {
			return m, nil // control key: ignore
		}
		m.cmSearchInput += msg.Text
	}
	m.cmQuery = m.cmSearchInput
	m.buildCommentList()
	m.cmIdx, m.cmScroll = 0, 0 // a new query starts from the first hit
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
	hidden := m.resolvedHidden()
	title := fmt.Sprintf(" Comments (%d threads · %d open)", len(m.cmThreads), open)
	switch {
	case m.cmQuery != "":
		title = fmt.Sprintf(" Comments (%d of %d match “%s”)", len(m.cmThreads), m.cmCandidates(), m.cmQuery)
		if hidden > 0 {
			title += fmt.Sprintf(" · %d resolved hidden", hidden)
		}
	case hidden > 0:
		title = fmt.Sprintf(" Comments (%d open · %d resolved hidden)", open, hidden)
	}
	lines := []string{padRight(truncate(styTitle.Render(title), width), width), styBorder.Render(strings.Repeat("─", width))}
	if m.cmSearching {
		lines = append(lines, padRight(truncate(styAccent.Render(" / ")+m.cmSearchInput+styAccent.Render("▏"), width), width))
	}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	if len(m.cmThreads) == 0 {
		msg := "  Every thread is resolved · " + m.keys.Label(keys.Comments, "show_resolved") + " shows them"
		if m.cmQuery != "" {
			msg = "  No thread matches “" + m.cmQuery + "” · " + m.keys.Label(keys.Comments, "close") + " clears the search"
		}
		lines = append(lines, padRight(truncate(styDim.Render(msg), width), width))
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
	prev, body := cmPreview(t, m.cmQuery)
	meta := ""
	if prev != nil {
		meta = fmt.Sprintf("  @%s · %s · %d comment(s) · ", prev.Author, ago(prev.CreatedAt), len(t.Comments))
	}
	l1 := " " + marker + highlightMatches(loc, m.cmQuery, base.Foreground(colText).Bold(true)) +
		highlightMatches(meta, m.cmQuery, base.Foreground(colDim)) + status

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

	l3 := base.Render("     ") + highlightMatches(body, m.cmQuery, base.Foreground(colText))
	return append(out, padRightBg(truncateTail(l3, width), width, bg), padRightBg("", width, bg))
}

// commentAtY maps a screen row to a comments-screen item index, or -1.
func (m *Model) commentAtY(y int) int {
	top := headerH + paneHeaderH
	if m.cmSearching {
		top++ // the / prompt sits above the list
	}
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
