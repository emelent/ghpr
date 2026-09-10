package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
)

// rowState describes how a row is highlighted.
type rowState int

const (
	stNormal rowState = iota
	stRange           // inside the visual selection
	stCursor          // under the cursor
)

// lineBg returns the background for a diff line kind in a given state.
func lineBg(k diff.Kind, st rowState) color.Color {
	switch k {
	case diff.Add:
		switch st {
		case stCursor:
			return colAddCurBg
		case stRange:
			return colAddSelBg
		}
		return colAddBg
	case diff.Del:
		switch st {
		case stCursor:
			return colDelCurBg
		case stRange:
			return colDelSelBg
		}
		return colDelBg
	}
	switch st {
	case stCursor:
		return colCtxCurBg
	case stRange:
		return colCtxSelBg
	}
	return lipgloss.NoColor{}
}

func signOf(k diff.Kind) (string, color.Color) {
	switch k {
	case diff.Add:
		return "+", colAddFg
	case diff.Del:
		return "-", colDelFg
	}
	return " ", colNumFg
}

func numStr(n, w int) string {
	if n <= 0 {
		return strings.Repeat(" ", w)
	}
	return fmt.Sprintf("%*d", w, n)
}

// renderUnified renders "old new ± content" for one line.
func (m *Model) renderUnified(l *diff.Line, st rowState, width, numW int) string {
	bg := lineBg(l.Kind, st)
	sign, signFg := signOf(l.Kind)
	gut := lipgloss.NewStyle().Background(bg).Foreground(colNumFg).Render(
		numStr(l.OldNum, numW) + " " + numStr(l.NewNum, numW) + " ")
	sg := lipgloss.NewStyle().Background(bg).Foreground(signFg).Bold(true).Render(sign + " ")
	gutW := numW*2 + 4
	content := renderSpans(m.spans[l], bg, width-gutW)
	return gut + sg + content
}

// renderHalf renders "num ± content" for one side of a split row.
func (m *Model) renderHalf(l *diff.Line, st rowState, width, numW int) string {
	if l == nil {
		bg := lineBg(diff.Context, st)
		return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width))
	}
	bg := lineBg(l.Kind, st)
	sign, signFg := signOf(l.Kind)
	n := l.NewNum
	if l.Kind == diff.Del {
		n = l.OldNum
	}
	gut := lipgloss.NewStyle().Background(bg).Foreground(colNumFg).Render(numStr(n, numW) + " ")
	sg := lipgloss.NewStyle().Background(bg).Foreground(signFg).Bold(true).Render(sign + " ")
	gutW := numW + 3
	return gut + sg + renderSpans(m.spans[l], bg, width-gutW)
}

// renderThread renders a review thread as a bordered block.
func renderThread(t *gh.Thread, selected bool, width int) []string {
	bg := color.Color(colThreadBg)
	if selected {
		bg = colThreadCu
	}
	borderCol := colWarn
	if t.IsResolved {
		borderCol = colOK
	}
	indent := lipgloss.NewStyle().Background(bg).Render("    ")
	bar := lipgloss.NewStyle().Background(bg).Foreground(borderCol).Render("┃ ")
	cw := width - 6
	if cw < 10 {
		cw = 10
	}
	base := lipgloss.NewStyle().Background(bg)
	var out []string
	emit := func(s string) {
		out = append(out, indent+bar+padRightBg(truncate(s, cw), cw, bg))
	}

	status := base.Foreground(colWarn).Render("○ open")
	if t.IsResolved {
		status = base.Foreground(colOK).Render("✓ resolved")
	}
	if t.IsOutdated {
		status += base.Foreground(colDim).Render(" · outdated")
	}
	loc := ""
	if t.Line > 0 {
		loc = fmt.Sprintf("%s:%d", t.DiffSide, t.Line)
		if t.StartLine > 0 && t.StartLine != t.Line {
			loc = fmt.Sprintf("%s:%d-%d", t.DiffSide, t.StartLine, t.Line)
		}
	} else if t.OriginalLine > 0 {
		loc = fmt.Sprintf("orig %d", t.OriginalLine)
	}
	head := base.Foreground(colDim).Render(fmt.Sprintf("Thread · %d comment(s) · %s  ", len(t.Comments), loc)) + status
	emit(head)

	for i, c := range t.Comments {
		prefix := ""
		if i > 0 {
			prefix = "↳ "
		}
		author := base.Foreground(colAccent).Bold(true).Render(prefix + "@" + c.Author)
		when := base.Foreground(colDim).Render("  " + ago(c.CreatedAt))
		emit(author + when)
		body := strings.TrimSpace(strings.ReplaceAll(c.Body, "\r", ""))
		if body == "" {
			body = "(empty)"
		}
		wrapped := ansi.Wrap(expandTabs(body), cw, "")
		for _, bl := range strings.Split(wrapped, "\n") {
			emit(base.Foreground(colText).Render(bl))
		}
		if i < len(t.Comments)-1 {
			emit("")
		}
	}
	return out
}

// renderRow renders a row to one or more full-width lines.
func (m *Model) renderRow(r *row, st rowState, width, numW int) []string {
	selected := st == stCursor
	switch r.kind {
	case rowHunk:
		st := styHunk
		if selected {
			st = styHunkCur
		}
		return []string{padRightBg(st.Render(truncateTail(r.hunk.Header, width)), width, st.GetBackground())}
	case rowNote:
		s := styNote.Render(truncateTail("  "+r.note, width))
		if selected {
			return []string{padRightBg(lipgloss.NewStyle().Background(colCtxCurBg).Render(truncateTail("  "+r.note, width)), width, colCtxCurBg)}
		}
		return []string{padRight(s, width)}
	case rowLine:
		return []string{m.renderUnified(r.line, st, width, numW)}
	case rowSplit:
		lw := (width - 1) / 2
		rw := width - 1 - lw
		sep := styBorder.Render("│")
		if st != stNormal {
			sep = lipgloss.NewStyle().Foreground(colBorder).Background(lineBg(diff.Context, st)).Render("│")
		}
		return []string{m.renderHalf(r.left, st, lw, numW) + sep + m.renderHalf(r.right, st, rw, numW)}
	case rowThread:
		return renderThread(r.thread, selected, width)
	}
	return []string{padRight("", width)}
}

// rowHeight returns the number of terminal lines a row occupies.
func (m *Model) rowHeight(i int) int {
	r := &m.rows[i]
	if r.kind != rowThread {
		return 1
	}
	if h, ok := m.threadH[i]; ok {
		return h
	}
	h := len(renderThread(r.thread, false, m.diffWidth()))
	m.threadH[i] = h
	return h
}

// ---------- panels ----------

func statusLetter(s diff.Status) string {
	switch s {
	case diff.Added:
		return styOK.Render("A")
	case diff.Deleted:
		return styErr.Render("D")
	case diff.Renamed:
		return styAccent.Render("R")
	}
	return styWarn.Render("M")
}

func (m *Model) threadCount(path string) (open, total int) {
	for _, t := range m.threads {
		if t.Path == path {
			total++
			if !t.IsResolved {
				open++
			}
		}
	}
	return
}

// renderFiles renders the file list panel.
func (m *Model) renderFiles(width, height int) []string {
	lines := make([]string, 0, height)
	title := styTitle.Render(fmt.Sprintf(" Files (%d)", len(m.files)))
	lines = append(lines, padRight(truncate(title, width), width))
	lines = append(lines, styBorder.Render(strings.Repeat("─", width)))
	avail := height - 2
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	// Keep selection visible.
	if m.fileIdx < m.fileScroll {
		m.fileScroll = m.fileIdx
	}
	if m.fileIdx >= m.fileScroll+avail {
		m.fileScroll = m.fileIdx - avail + 1
	}
	for i := m.fileScroll; i < len(m.files) && len(lines) < height; i++ {
		f := &m.files[i]
		open, total := m.threadCount(f.Path())
		badge := ""
		if total > 0 {
			if open > 0 {
				badge = styWarn.Render(fmt.Sprintf(" ●%d", open))
			} else {
				badge = styOK.Render(fmt.Sprintf(" ✓%d", total))
			}
		}
		counts := styOK.Render(fmt.Sprintf("+%d", f.Additions)) + " " + styErr.Render(fmt.Sprintf("-%d", f.Deletions))
		tail := badge + " " + counts
		pathW := width - 3 - ansi.StringWidth(tail)
		if pathW < 4 {
			pathW = 4
			tail = ""
		}
		name := leftEllipsis(f.Path(), pathW)
		line := " " + statusLetter(f.Status) + " " + padRight(name, pathW) + tail
		line = padRight(line, width)
		if i == m.fileIdx {
			if m.filesFocused {
				line = styFileSel.Render(ansi.Strip(line))
			} else {
				line = styFileSelD.Render(ansi.Strip(line))
			}
		}
		lines = append(lines, line)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// renderDiff renders the diff pane (file title + visible rows).
func (m *Model) renderDiff(width, height int) []string {
	lines := make([]string, 0, height)
	if len(m.files) == 0 {
		lines = append(lines, padRight(styNote.Render("  No files in diff."), width))
	} else {
		f := &m.files[m.fileIdx]
		mode := "inline"
		if m.split {
			mode = "side-by-side"
		}
		hdr := styTitle.Render(" "+f.Path()) + styDim.Render(fmt.Sprintf("  %s · %s", f.Status, mode))
		if f.Status == diff.Renamed {
			hdr = styTitle.Render(" "+f.OldPath+" → "+f.NewPath) + styDim.Render(fmt.Sprintf("  renamed · %s", mode))
		}
		lines = append(lines, padRight(truncateTail(hdr, width), width))
	}
	lines = append(lines, styBorder.Render(strings.Repeat("─", width)))
	avail := height - 2
	if avail <= 0 || len(m.rows) == 0 {
		for len(lines) < height {
			lines = append(lines, strings.Repeat(" ", width))
		}
		return lines
	}
	m.ensureCursorVisible(avail)

	// Find first row intersecting the scroll offset.
	start := 0
	for start < len(m.rows) && m.rowStart[start]+m.rowHeight(start) <= m.scroll {
		start++
	}
	numW := m.numW
	y := 0
	for i := start; i < len(m.rows) && y < avail; i++ {
		st := stNormal
		switch {
		case i == m.cursor:
			st = stCursor
		case m.inSelection(i):
			st = stRange
		}
		rendered := m.renderRow(&m.rows[i], st, width, numW)
		skip := 0
		if m.rowStart[i] < m.scroll {
			skip = m.scroll - m.rowStart[i]
		}
		for k := skip; k < len(rendered) && y < avail; k++ {
			lines = append(lines, rendered[k])
			y++
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// renderHeader renders the two-line PR header.
func (m *Model) renderHeader(width int) []string {
	if m.pr == nil {
		return []string{padRight(styTitle.Render(fmt.Sprintf(" %s #%d", m.client.Repo, m.number)), width), padRight(styDim.Render(" loading…"), width)}
	}
	pr := m.pr
	state := styOK.Render(pr.State)
	switch pr.State {
	case "MERGED":
		state = styAccent.Render("MERGED")
	case "CLOSED":
		state = styErr.Render("CLOSED")
	}
	if pr.IsDraft {
		state += styDim.Render(" DRAFT")
	}
	l1 := styTitle.Render(fmt.Sprintf(" #%d %s", pr.Number, pr.Title)) + "  " + state
	decision := pr.ReviewDecision
	if decision == "" {
		decision = "no review"
	}
	decStyled := styDim.Render(decision)
	switch decision {
	case "APPROVED":
		decStyled = styOK.Render(decision)
	case "CHANGES_REQUESTED":
		decStyled = styErr.Render(decision)
	case "REVIEW_REQUIRED":
		decStyled = styWarn.Render(decision)
	}
	open, total := 0, 0
	for _, t := range m.threads {
		total++
		if !t.IsResolved {
			open++
		}
	}
	l2 := styDim.Render(fmt.Sprintf(" %s · @%s · %s → %s · ", m.client.Repo, pr.Author.Login, pr.HeadRefName, pr.BaseRefName)) +
		styOK.Render(fmt.Sprintf("+%d", pr.Additions)) + " " + styErr.Render(fmt.Sprintf("-%d", pr.Deletions)) +
		styDim.Render(fmt.Sprintf(" · %d files · threads %d open / %d · ", pr.ChangedFiles, open, total)) + decStyled
	return []string{padRight(truncateTail(l1, width), width), padRight(truncateTail(l2, width), width)}
}

// renderStatus renders the bottom bar.
func (m *Model) renderStatus(width int) string {
	var left string
	switch {
	case m.busy != "":
		left = styBar.Render(" "+m.spinner.View()+" ") + styBar.Render(m.busy)
	case m.overlay == overlayReview:
		left = styBar.Render(" Submit review: ") + styBarKey.Render("a") + styBar.Render(" approve  ") +
			styBarKey.Render("r") + styBar.Render(" request changes  ") + styBarKey.Render("c") + styBar.Render(" comment  ") +
			styBarKey.Render("esc") + styBar.Render(" cancel")
	case m.selecting && m.overlay == overlayNone:
		left = styBar.Render(fmt.Sprintf(" %d line(s) selected  ", m.selectedLineCount())) +
			styBarKey.Render("j/k") + styBar.Render(" extend  ") +
			styBarKey.Render("c") + styBar.Render(" comment  ") +
			styBarKey.Render("esc") + styBar.Render(" cancel")
	case m.status != "":
		if m.statusErr {
			left = lipgloss.NewStyle().Background(colBarBg).Foreground(colErr).Render(" ✗ " + m.status)
		} else {
			left = lipgloss.NewStyle().Background(colBarBg).Foreground(colOK).Render(" ✓ " + m.status)
		}
	default:
		left = styBar.Render(" ")
	}
	var right string
	if m.overlay == overlayInput {
		right = styBarKey.Render("⌘+enter") + styBarDim.Render(" submit  ") + styBarKey.Render("esc") + styBarDim.Render(" cancel ")
	} else {
		hints := []struct{ k, v string }{{"j/k", "move"}, {"s", "split"}, {"V", "select"}, {"c", "comment"}, {"r", "reply"}, {"x", "resolve"}, {"v", "review"}, {"?", "help"}}
		var sb strings.Builder
		for _, h := range hints {
			sb.WriteString(styBarKey.Render(h.k) + styBarDim.Render(" "+h.v+"  "))
		}
		right = sb.String()
	}
	lw := ansi.StringWidth(left)
	rw := ansi.StringWidth(right)
	if lw+rw > width {
		right = ""
		rw = 0
	}
	fill := width - lw - rw
	if fill < 0 {
		return truncate(left, width)
	}
	return left + styBar.Render(strings.Repeat(" ", fill)) + right
}

// renderInput renders the comment/review text entry panel.
func (m *Model) renderInput(width, height int) []string {
	lines := []string{styBorder.Render(strings.Repeat("─", width))}
	lines = append(lines, padRight(truncateTail(styInputTtl.Render(" "+m.inputTitle), width), width))
	for _, l := range strings.Split(m.ta.View(), "\n") {
		lines = append(lines, padRight(l, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}

func (m *Model) renderHelp(width, height int) []string {
	rows := [][2]string{
		{"j / k, ↓ / ↑", "move cursor"},
		{"ctrl+d / ctrl+u, pgdn / pgup", "half page"},
		{"g / G", "top / bottom"},
		{"] / [", "next / previous file"},
		{"n / N", "next / previous review thread"},
		{"tab", "focus file list / diff"},
		{"f", "toggle file list"},
		{"s", "toggle inline / side-by-side"},
		{"c", "comment on the current line (or selection)"},
		{"V", "start / stop selecting lines for a multi-line comment"},
		{"r", "reply to the thread under the cursor"},
		{"x", "resolve / unresolve the thread under the cursor"},
		{"v", "submit a review (approve / request changes / comment)"},
		{"C", "comment on the PR (general)"},
		{"o", "open the PR in the browser"},
		{"R", "refresh PR, diff and threads"},
		{"⌘+enter / esc", "submit / cancel text entry (ctrl+enter also submits)"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	lines := []string{padRight(styTitle.Render(" Keys"), width), styBorder.Render(strings.Repeat("─", width))}
	for _, r := range rows {
		lines = append(lines, padRight("  "+styHelpKey.Render(padRight(r[0], 30))+styDim.Render(r[1]), width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}
