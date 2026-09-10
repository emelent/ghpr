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
	content := renderSpans(m.spans[l], bg, width-gutW, m.hscroll)
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
	return gut + sg + renderSpans(m.spans[l], bg, width-gutW, m.hscroll)
}

// renderThread renders a review thread as a bordered block.
func renderThread(t *gh.Thread, selected bool, width int, target *gh.Comment) []string {
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
		if target != nil && &t.Comments[i] == target {
			when += base.Foreground(colErr).Bold(true).Render("  ✗ delete?")
		}
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
		return renderThread(r.thread, selected, width, m.deleteTarget())
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
	h := len(renderThread(r.thread, false, m.diffWidth(), nil))
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
	if m.store != nil && len(m.files) > 0 {
		title += styDim.Render(fmt.Sprintf(" · %d viewed", m.viewedCount()))
	}
	lines = append(lines, padRight(truncate(title, width), width))
	lines = append(lines, styBorder.Render(strings.Repeat("─", width)))
	avail := height - 2
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	if m.tree {
		return m.renderTree(lines, width, height, avail)
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
		viewed := m.isViewed(f.Path())
		mark := " "
		if viewed {
			mark = styOK.Render("✓")
		}
		pathW--
		name := leftEllipsis(f.Path(), pathW)
		if viewed {
			name = styDim.Render(name)
		}
		line := mark + statusLetter(f.Status) + " " + padRight(name, pathW) + tail
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

// renderTree renders the file panel as a directory tree.
func (m *Model) renderTree(lines []string, width, height, avail int) []string {
	m.rebuildTree()
	sel := m.treeIndex()
	if sel < m.fileScroll {
		m.fileScroll = sel
	}
	if sel >= m.fileScroll+avail {
		m.fileScroll = sel - avail + 1
	}
	for i := m.fileScroll; i < len(m.treeNodes) && len(lines) < height; i++ {
		n := &m.treeNodes[i]
		indent := strings.Repeat("  ", n.depth)
		var line string
		if n.isDir {
			arrow := "▾ "
			if m.collapsed[n.path] {
				arrow = "▸ "
			}
			tail := ""
			if m.collapsed[n.path] {
				tail = styDim.Render(fmt.Sprintf(" %d", n.files))
				if n.viewed == n.files {
					tail = styOK.Render(fmt.Sprintf(" ✓%d", n.files))
				} else if n.viewed > 0 {
					tail = styDim.Render(fmt.Sprintf(" %d/%d", n.viewed, n.files))
				}
				tail += " " + styOK.Render(fmt.Sprintf("+%d", n.adds)) + " " + styErr.Render(fmt.Sprintf("-%d", n.dels))
			}
			nameW := width - 1 - len(indent) - 2 - ansi.StringWidth(tail)
			if nameW < 4 {
				nameW, tail = 4, ""
			}
			name := styAccent.Render(truncateTail(n.label+"/", nameW))
			line = " " + indent + styDim.Render(arrow) + padRight(name, nameW) + tail
		} else {
			f := &m.files[n.fileIdx]
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
			viewed := m.isViewed(f.Path())
			mark := " "
			if viewed {
				mark = styOK.Render("✓")
			}
			nameW := width - len(indent) - 3 - ansi.StringWidth(tail)
			if nameW < 4 {
				nameW, tail = 4, ""
			}
			name := truncateTail(n.label, nameW)
			if viewed {
				name = styDim.Render(name)
			}
			line = mark + indent + statusLetter(f.Status) + " " + padRight(name, nameW) + tail
		}
		line = padRight(line, width)
		if i == sel {
			if m.filesFocused {
				line = styFileSel.Render(ansi.Strip(line))
			} else if !n.isDir {
				line = styFileSelD.Render(ansi.Strip(line))
			}
		} else if !n.isDir && n.fileIdx == m.fileIdx && m.filesFocused {
			line = styFileSelD.Render(ansi.Strip(line))
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
		if v, ok := m.viewedInfo(f.Path()); ok {
			mode += " · viewed " + ago(v.ViewedAt)
		}
		if m.hscroll > 0 {
			mode += fmt.Sprintf(" · → col %d", m.hscroll+1)
		}
		switch {
		case m.showingFull():
			mode += " · full file"
		case m.full && m.fullPending[m.fileIdx]:
			mode += " · loading full file…"
		case m.full:
			mode += " · hunks only"
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
	mergeInfo := ""
	if pr.State == "OPEN" && pr.MergeStateStatus != "" {
		st := styDim.Render(pr.MergeStateStatus)
		switch pr.MergeStateStatus {
		case "CLEAN":
			st = styOK.Render("mergeable")
		case "BLOCKED", "DIRTY", "UNSTABLE", "BEHIND":
			st = styErr.Render(pr.MergeStateStatus)
		case "DRAFT", "HAS_HOOKS":
			st = styWarn.Render(pr.MergeStateStatus)
		}
		mergeInfo = styDim.Render(" · ") + st
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
		styDim.Render(fmt.Sprintf(" · %d files · threads %d open / %d · ", pr.ChangedFiles, open, total)) + decStyled + mergeInfo
	return []string{padRight(truncateTail(l1, width), width), padRight(truncateTail(l2, width), width)}
}

// renderStatus renders the bottom bar.
func (m *Model) renderStatus(width int) string {
	var left string
	switch {
	case m.busy != "":
		left = styBar.Render(" "+m.spinner.View()+" ") + styBar.Render(m.busy)
	case m.overlay == overlayMerge:
		del := "off"
		if m.mergeDelete {
			del = "on"
		}
		if m.mergeMethod == gh.Rebase {
			left = styBar.Render(fmt.Sprintf(" Rebase and merge PR #%d (delete branch: %s)? ", m.number, del)) +
				styBarKey.Render("y") + styBar.Render(" confirm  ") + styBarKey.Render("n") + styBar.Render(" cancel")
		} else {
			warn := ""
			if m.pr != nil && m.pr.MergeStateStatus != "" && m.pr.MergeStateStatus != "CLEAN" && m.pr.MergeStateStatus != "UNKNOWN" {
				warn = lipgloss.NewStyle().Background(colBarBg).Foreground(colWarn).Render(" ⚠ "+m.pr.MergeStateStatus) + styBar.Render("  ")
			}
			left = styBar.Render(" Merge: ") + warn + styBarKey.Render("m") + styBar.Render(" merge commit  ") +
				styBarKey.Render("s") + styBar.Render(" squash  ") + styBarKey.Render("r") + styBar.Render(" rebase  ") +
				styBarKey.Render("d") + styBar.Render(" delete branch: "+del+"  ") + styBarKey.Render("esc") + styBar.Render(" cancel")
		}
	case m.overlay == overlayDelete:
		c := m.deleteTarget()
		snippet := strings.Join(strings.Fields(c.Body), " ")
		if len(snippet) > 40 {
			snippet = snippet[:40] + "…"
		}
		pick := ""
		if len(m.delChoices) > 1 {
			pick = styBarKey.Render("j/k") + styBar.Render(fmt.Sprintf(" choose (%d/%d)  ", m.delIdx+1, len(m.delChoices)))
		}
		left = styBar.Render(fmt.Sprintf(" Delete @%s's comment “%s”? ", c.Author, snippet)) +
			styBarKey.Render("y") + styBar.Render(" confirm  ") + pick + styBarKey.Render("n") + styBar.Render(" cancel")
	case m.overlay == overlayState:
		if m.pr != nil && m.pr.State == "OPEN" {
			del := "off"
			if m.mergeDelete {
				del = "on"
			}
			left = styBar.Render(fmt.Sprintf(" Close PR #%d without merging? ", m.number)) + styBarKey.Render("y") + styBar.Render(" confirm  ") +
				styBarKey.Render("d") + styBar.Render(" delete branch: "+del+"  ") + styBarKey.Render("n") + styBar.Render(" cancel")
		} else {
			left = styBar.Render(fmt.Sprintf(" Reopen PR #%d? ", m.number)) + styBarKey.Render("y") + styBar.Render(" confirm  ") +
				styBarKey.Render("n") + styBar.Render(" cancel")
		}
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
	switch {
	case m.overlay == overlayInput && m.inKind == inputMerge:
		right = styBarKey.Render("⌘+j") + styBarDim.Render(" merge  ") + styBarKey.Render("esc") + styBarDim.Render(" cancel ")
	case m.overlay == overlayInput:
		right = styBarKey.Render("⌘+j") + styBarDim.Render(" submit  ") + styBarKey.Render("esc") + styBarDim.Render(" cancel ")
	case m.screen == screenPicker:
		right = styBarKey.Render("enter/l") + styBarDim.Render(" open  ") + styBarKey.Render("s") + styBarDim.Render(" state: "+m.listState+"  ") +
			styBarKey.Render("/") + styBarDim.Render(" filter  ") + styBarKey.Render("q") + styBarDim.Render(" quit ")
	default:
		hints := []struct{ k, v string }{{"j/k", "move"}, {"J/K", "change"}, {"m", "viewed"}, {"s", "split"}, {"F", "full"}, {"V", "select"}, {"c", "comment"}, {"r", "reply"}, {"x", "resolve"}, {"v", "review"}, {"M", "merge"}, {"?", "help"}}
		var sb strings.Builder
		for _, h := range hints {
			sb.WriteString(styBarKey.Render(h.k) + styBarDim.Render(" "+h.v+"  "))
		}
		right = sb.String()
	}
	if m.debugKeys && m.lastKey != "" {
		right = lipgloss.NewStyle().Background(colBarBg).Foreground(colWarn).Bold(true).Render("key: "+m.lastKey) + styBar.Render("  ") + right
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
		{"ctrl+f / ctrl+b", "full page"},
		{"g / G", "top / bottom (diff, file list or PR list)"},
		{"] / [", "next / previous file"},
		{"J / K", "next / previous change in the file"},
		{"n / N", "next / previous review thread"},
		{"tab", "focus file list / diff"},
		{"h / l", "diff: scroll left / right when lines overflow; h at the left edge goes to the file tree"},
		{"l (file tree)", "open the selected file"},
		{"f", "toggle file list"},
		{"t", "file list: tree / flat"},
		{"enter / l, h, H / L", "file tree: open file or expand dir, collapse (or go to parent), collapse / expand all"},
		{"s", "toggle inline / side-by-side"},
		{"F", "toggle full file view (whole file with changes in place)"},
		{"m", "mark / unmark the file as viewed (auto-unmarked if it changes later)"},
		{"c", "comment on the current line (or selection)"},
		{"V", "start / stop selecting lines for a multi-line comment"},
		{"r", "reply to the thread under the cursor"},
		{"x", "resolve / unresolve the thread under the cursor"},
		{"d", "delete one of your comments in the thread under the cursor (y to confirm)"},
		{"v", "submit a review (approve / request changes / comment)"},
		{"M", "merge the PR: m / s open the commit message to edit, r rebase, d delete branch"},
		{"X", "close the PR without merging, or reopen a closed PR"},
		{"C", "comment on the PR (general)"},
		{"o", "open the PR in the browser"},
		{"R", "refresh PR, diff and threads"},
		{"⌘+j / esc", "submit / cancel text entry (ctrl+s also submits)"},
		{"?", "toggle this help"},
		{"b / backspace", "back to the pull request list"},
		{"q", "back to the list when opened from it, otherwise quit"},
		{"Q / ctrl+c", "quit"},
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
