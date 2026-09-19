package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
	"ghpr/internal/keys"
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
	gut := m.gutterStyle(l, bg).Render(numStr(l.OldNum, numW) + " " + numStr(l.NewNum, numW) + " ")
	sg := lipgloss.NewStyle().Background(bg).Foreground(signFg).Bold(true).Render(sign + " ")
	gutW := numW*2 + 4
	content := renderSpans(m.lineSpans(l), bg, width-gutW, m.hscroll)
	return gut + sg + content
}

// gutterStyle styles line numbers; a line with a note (a) gets the note colour.
func (m *Model) gutterStyle(l *diff.Line, bg color.Color) lipgloss.Style {
	st := lipgloss.NewStyle().Background(bg).Foreground(colNumFg)
	if m.noteIndexForLine(l) >= 0 {
		st = st.Foreground(colWarn).Bold(true)
	}
	return st
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
	gut := m.gutterStyle(l, bg).Render(numStr(n, numW) + " ")
	sg := lipgloss.NewStyle().Background(bg).Foreground(signFg).Bold(true).Render(sign + " ")
	gutW := numW + 3
	return gut + sg + renderSpans(m.lineSpans(l), bg, width-gutW, m.hscroll)
}

// renderThread renders a review thread. target, when non-nil, is the comment
// currently selected in the picker and is flagged with mark.
func renderThread(t *gh.Thread, selected bool, width int, target *gh.Comment, mark string) []string {
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
			when += base.Foreground(colErr).Bold(true).Render(mark)
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
		mark := "  ✗ delete?"
		if m.overlay == overlayEdit {
			mark = "  ✎ edit?"
		}
		return renderThread(r.thread, selected, width, m.pickTarget(), mark)
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
	h := len(renderThread(r.thread, false, m.diffWidth(), nil, ""))
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
	if m.threadsOnly {
		title = styTitle.Render(fmt.Sprintf(" Files (%d of %d)", len(m.shownFiles()), len(m.files))) + styWarn.Render(" · with comments")
	}
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
	shown := m.shownFiles()
	pos := 0
	for k, i := range shown {
		if i == m.fileIdx {
			pos = k
		}
	}
	if pos < m.fileScroll {
		m.fileScroll = pos
	}
	if pos >= m.fileScroll+avail {
		m.fileScroll = pos - avail + 1
	}
	for k := m.fileScroll; k < len(shown) && len(lines) < height; k++ {
		i := shown[k]
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
	// Who approved and who asked for changes, by their latest review.
	if approved, changes := gh.Approvers(pr.LatestReviews); len(approved)+len(changes) > 0 {
		if len(approved) > 0 {
			decStyled += styOK.Render(" ✓ " + strings.Join(approved, ", "))
		}
		if len(changes) > 0 {
			decStyled += styErr.Render(" ✗ " + strings.Join(changes, ", "))
		}
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
				m.hk(keys.Merge, "confirm") + styBar.Render(" confirm  ") + m.hk(keys.Merge, "cancel") + styBar.Render(" cancel")
		} else {
			warn := ""
			if m.pr != nil && m.pr.MergeStateStatus != "" && m.pr.MergeStateStatus != "CLEAN" && m.pr.MergeStateStatus != "UNKNOWN" {
				warn = lipgloss.NewStyle().Background(colBarBg).Foreground(colWarn).Render(" ⚠ "+m.pr.MergeStateStatus) + styBar.Render("  ")
			}
			left = styBar.Render(" Merge: ") + warn + m.hk(keys.Merge, "merge_commit") + styBar.Render(" merge commit  ") +
				m.hk(keys.Merge, "squash") + styBar.Render(" squash  ") + m.hk(keys.Merge, "rebase") + styBar.Render(" rebase  ") +
				m.hk(keys.Merge, "delete_branch") + styBar.Render(" delete branch: "+del+"  ") + m.hk(keys.Merge, "cancel") + styBar.Render(" cancel")
		}
	case m.overlay == overlayDelete || m.overlay == overlayEdit:
		c := m.pickTarget()
		snippet := strings.Join(strings.Fields(c.Body), " ")
		if len(snippet) > 40 {
			snippet = snippet[:40] + "…"
		}
		pick := ""
		if len(m.pickChoices) > 1 {
			pick = m.hk2(keys.Pick, "down", "up") + styBar.Render(fmt.Sprintf(" choose (%d/%d)  ", m.pickIdx+1, len(m.pickChoices)))
		}
		verb, confirm := "Delete", "confirm"
		if m.overlay == overlayEdit {
			verb, confirm = "Edit", "open editor"
		}
		left = styBar.Render(fmt.Sprintf(" %s @%s's comment “%s”? ", verb, c.Author, snippet)) +
			m.hk(keys.Pick, "confirm") + styBar.Render(" "+confirm+"  ") + pick + m.hk(keys.Pick, "cancel") + styBar.Render(" cancel")
	case m.overlay == overlayState:
		if m.pr != nil && m.pr.State == "OPEN" {
			del := "off"
			if m.mergeDelete {
				del = "on"
			}
			left = styBar.Render(fmt.Sprintf(" Close PR #%d without merging? ", m.number)) + m.hk(keys.Confirm, "confirm") + styBar.Render(" confirm  ") +
				m.hk(keys.Confirm, "delete_branch") + styBar.Render(" delete branch: "+del+"  ") + m.hk(keys.Confirm, "cancel") + styBar.Render(" cancel")
		} else {
			left = styBar.Render(fmt.Sprintf(" Reopen PR #%d? ", m.number)) + m.hk(keys.Confirm, "confirm") + styBar.Render(" confirm  ") +
				m.hk(keys.Confirm, "cancel") + styBar.Render(" cancel")
		}
	case m.overlay == overlayReview:
		left = styBar.Render(" Submit review: ") + m.hk(keys.Review, "approve") + styBar.Render(" approve  ") +
			m.hk(keys.Review, "request_changes") + styBar.Render(" request changes  ") + m.hk(keys.Review, "comment") + styBar.Render(" comment  ") +
			m.hk(keys.Review, "cancel") + styBar.Render(" cancel")
	case m.overlay == overlayFiles:
		left = styBar.Render(" Go to file: type to filter  ") + m.hk2(keys.Prompt, "up", "down") + styBar.Render(" choose  ") +
			m.hk(keys.Prompt, "accept") + styBar.Render(" open  ") + m.hk(keys.Prompt, "cancel") + styBar.Render(" cancel")
	case m.overlay == overlayGlobal:
		left = styBar.Render(" Search all files: type to search  ") + m.hk2(keys.Prompt, "up", "down") + styBar.Render(" choose  ") +
			m.hk(keys.Prompt, "accept") + styBar.Render(" jump  ") + m.hk(keys.Prompt, "cancel") + styBar.Render(" cancel")
	case m.overlay == overlayNote:
		left = styBar.Render(" ⚑ Add note at "+m.ntPending.location()+": "+m.ntInput) + styBarKey.Render("▏") +
			styBar.Render("  ") + m.hk(keys.Prompt, "accept") + styBar.Render(" save  ") + m.hk(keys.Prompt, "cancel") + styBar.Render(" cancel")
	case m.overlay == overlaySearch:
		count := ""
		if m.searchInput != "" {
			_, total := m.matchPos()
			count = styBarDim.Render(fmt.Sprintf("  %d match(es)  ", total))
		}
		left = styBar.Render(" /"+m.searchInput) + styBarKey.Render("▏") + count +
			styBar.Render("  ") + m.hk(keys.Prompt, "accept") + styBar.Render(" keep  ") + m.hk(keys.Prompt, "cancel") + styBar.Render(" cancel")
	case m.screen == screenComments && m.cmSearching:
		left = styBar.Render(" /"+m.cmSearchInput) + styBarKey.Render("▏") +
			styBarDim.Render(fmt.Sprintf("  %d thread(s)  ", len(m.cmThreads))) +
			m.hk(keys.Prompt, "accept") + styBar.Render(" keep  ") + m.hk(keys.Prompt, "cancel") + styBar.Render(" cancel")
	case m.selecting && m.overlay == overlayNone:
		left = styBar.Render(fmt.Sprintf(" %d line(s) selected  ", m.selectedLineCount())) +
			m.hk2(keys.Diff, "down", "up") + styBar.Render(" extend  ") +
			m.hk(keys.Diff, "comment") + styBar.Render(" comment  ") +
			m.hk(keys.Diff, "cancel") + styBar.Render(" cancel")
	case m.status != "":
		if m.statusErr {
			left = lipgloss.NewStyle().Background(colBarBg).Foreground(colErr).Render(" ✗ " + m.status)
		} else {
			left = lipgloss.NewStyle().Background(colBarBg).Foreground(colOK).Render(" ✓ " + m.status)
		}
	case m.overlay == overlayNone && m.screen == screenDiff && m.rowNoteIndex(m.cursor) >= 0:
		nt := m.notes[m.rowNoteIndex(m.cursor)]
		left = lipgloss.NewStyle().Background(colBarBg).Foreground(colWarn).Bold(true).Render(" ⚑ "+nt.body) +
			styBar.Render("  ") + m.hk(keys.Diff, "note") + styBar.Render(" remove note  ") + m.hk(keys.Diff, "notes") + styBar.Render(fmt.Sprintf(" all notes (%d)", len(m.notes)))
	case m.screen == screenComments && m.cmQuery != "" && m.overlay == overlayNone:
		left = styBar.Render(" /"+m.cmQuery) + styBarDim.Render(fmt.Sprintf("  %d of %d thread(s)  ", len(m.cmThreads), m.cmCandidates())) +
			m.hk(keys.Comments, "close") + styBar.Render(" clear")
	case m.searchQ != "" && m.overlay == overlayNone:
		pos, total := m.matchPos()
		where := fmt.Sprintf("%d match(es)", total)
		if pos > 0 {
			where = fmt.Sprintf("%d/%d", pos, total)
		}
		left = styBar.Render(" /"+m.searchQ) + styBarDim.Render("  "+where+"  ") +
			m.hk2(keys.Diff, "next", "prev") + styBar.Render(" next / prev  ") + m.hk(keys.Diff, "cancel") + styBar.Render(" clear")
	default:
		left = styBar.Render(" ")
	}
	var right string
	switch {
	case m.overlay == overlayInput && m.inKind == inputMerge:
		right = m.hk(keys.Input, "submit") + styBarDim.Render(" merge  ") + m.hk(keys.Input, "cancel") + styBarDim.Render(" cancel ")
	case m.overlay == overlayInput:
		right = m.hk(keys.Input, "submit") + styBarDim.Render(" submit  ") + m.hk(keys.Input, "cancel") + styBarDim.Render(" cancel ")
	case m.screen == screenComments:
		resolved := " resolved  "
		if m.cmShowResolved {
			resolved = " hide resolved  "
		}
		right = m.hk2(keys.Comments, "down", "up") + styBarDim.Render(" move  ") + m.hk(keys.Comments, "open") + styBarDim.Render(" open in diff  ") +
			m.hk(keys.Comments, "reply") + styBarDim.Render(" reply  ") + m.hk(keys.Comments, "resolve") + styBarDim.Render(" resolve  ") +
			m.hk(keys.Comments, "search") + styBarDim.Render(" search  ") +
			m.hk(keys.Comments, "show_resolved") + styBarDim.Render(resolved) + m.hk(keys.Comments, "close") + styBarDim.Render(" close ")
	case m.screen == screenNotes:
		right = m.hk2(keys.Notes, "down", "up") + styBarDim.Render(" move  ") + m.hk(keys.Notes, "open") + styBarDim.Render(" go to line  ") +
			m.hk(keys.Notes, "delete") + styBarDim.Render(" remove  ") + m.hk(keys.Notes, "close") + styBarDim.Render(" close ")
	case m.screen == screenPicker:
		right = styBarKey.Render(m.keys.Label(keys.List, "open")) + styBarDim.Render(" open  ") + m.hk(keys.List, "cycle_state") + styBarDim.Render(" state: "+m.listState+"  ") +
			styBarKey.Render("/") + styBarDim.Render(" filter  ") + m.hk(keys.List, "quit") + styBarDim.Render(" quit ")
	default:
		hints := []struct{ k, v string }{
			{m.keys.First(keys.Diff, "down") + "/" + m.keys.First(keys.Diff, "up"), "move"},
			{m.keys.First(keys.Diff, "next_change") + "/" + m.keys.First(keys.Diff, "prev_change"), "change"},
			{m.keys.First(keys.Diff, "viewed"), "viewed"}, {m.keys.First(keys.Diff, "split"), "split"}, {m.keys.First(keys.Diff, "full_file"), "full"},
			{m.keys.First(keys.Diff, "select"), "select"}, {m.keys.First(keys.Diff, "comment"), "comment"}, {m.keys.First(keys.Diff, "reply"), "reply"},
			{m.keys.First(keys.Diff, "resolve"), "resolve"}, {m.keys.First(keys.Diff, "review"), "review"}, {m.keys.First(keys.Diff, "merge"), "merge"},
			{m.keys.First(keys.Diff, "help"), "help"},
		}
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

// hk renders the primary key of an action for a status-bar hint.
func (m *Model) hk(ctx, action string) string { return styBarKey.Render(m.keys.First(ctx, action)) }

// hk2 renders two actions' primary keys as "a/b".
func (m *Model) hk2(ctx, a, b string) string {
	return styBarKey.Render(m.keys.First(ctx, a) + "/" + m.keys.First(ctx, b))
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

// helpRow is one line of the help screen: the actions whose keys are shown
// (joined with " / ") and what they do. literal overrides the key column.
type helpRow struct {
	acts    [][2]string // {context, action}
	literal string
	desc    string
}

func (m *Model) renderHelp(width, height int) []string {
	d := func(actions ...string) [][2]string {
		out := make([][2]string, len(actions))
		for i, a := range actions {
			out[i] = [2]string{keys.Diff, a}
		}
		return out
	}
	f := func(actions ...string) [][2]string {
		out := make([][2]string, len(actions))
		for i, a := range actions {
			out[i] = [2]string{keys.Files, a}
		}
		return out
	}
	rows := []helpRow{
		{acts: d("down", "up"), desc: "move cursor"},
		{acts: d("half_page_down", "half_page_up"), desc: "half page"},
		{acts: d("page_down", "page_up"), desc: "full page"},
		{acts: d("top", "bottom"), desc: "top / bottom (diff, file list or PR list)"},
		{acts: d("next_file", "prev_file"), desc: "next / previous file"},
		{acts: d("next_change", "prev_change"), desc: "next / previous change in the file"},
		{acts: d("next", "prev"), desc: "next / previous review thread; next / previous match while a search is active"},
		{acts: d("search"), desc: "search the file (case-insensitive); enter keeps the match, esc cancels; in the file list it opens the file picker"},
		{acts: d("search_all"), desc: "search every file in the diff, exact hits first then fuzzy: ↑/↓ (ctrl+j/k) choose, enter jumps to the line"},
		{acts: d("file_picker"), desc: "fuzzy-find a file by path: type to filter, ↑/↓ (ctrl+j/k) choose, enter open"},
		{acts: d("cancel"), desc: "cancel the line selection, otherwise clear the search"},
		{acts: d("focus"), desc: "focus file list / diff"},
		{acts: d("scroll_left", "scroll_right"), desc: "diff: scroll left / right when lines overflow; left at the edge goes to the file tree (or back to the comments / notes list)"},
		{acts: f("open", "expand"), desc: "file tree: open the file or toggle / expand the directory"},
		{acts: f("collapse"), desc: "file tree: collapse the directory or go to its parent"},
		{acts: f("collapse_all", "expand_all"), desc: "file tree: collapse / expand every directory"},
		{acts: d("toggle_files"), desc: "toggle file list"},
		{acts: d("tree_flat"), desc: "file list: tree / flat"},
		{acts: d("threads_only"), desc: "show only files with review threads (file list, next / previous file, file picker and search follow it)"},
		{acts: d("comments"), desc: "comments screen: open threads with their code line and first comment; j/k move, l opens it in the diff (h there comes back), r replies to it in a panel under the list, x resolves / unresolves, t shows resolved threads too, / filters by path, author or comment text (esc clears it), esc closes"},
		{acts: d("note"), desc: "add a note on the current line to come back to (press again to remove it); noted lines get amber line numbers; notes are kept per PR"},
		{acts: d("notes"), desc: "notes screen: your notes with their lines; j/k move, l jumps there (h comes back), d removes, esc closes"},
		{acts: d("split"), desc: "toggle inline / side-by-side"},
		{acts: d("full_file"), desc: "toggle full file view (whole file with changes in place)"},
		{acts: d("viewed"), desc: "mark / unmark the file as viewed; on a folder in the file tree, every file beneath it (auto-unmarked if a file changes later; synced with GitHub)"},
		{acts: d("comment"), desc: "comment on the current line (or selection)"},
		{acts: d("select"), desc: "start / stop selecting lines for a multi-line comment"},
		{acts: d("reply"), desc: "reply to the thread under the cursor"},
		{acts: d("resolve"), desc: "resolve / unresolve the thread under the cursor"},
		{acts: d("delete_comment"), desc: "delete one of your comments in the thread under the cursor (y to confirm)"},
		{acts: d("edit_comment"), desc: "edit one of your comments in the thread under the cursor (y opens the editor)"},
		{acts: d("review"), desc: "submit a review (approve / request changes / comment)"},
		{acts: d("merge"), desc: "merge the PR: m / s open the commit message to edit, r rebase, d delete branch"},
		{acts: d("close_reopen"), desc: "close the PR without merging, or reopen a closed PR"},
		{acts: d("pr_comment"), desc: "comment on the PR (general)"},
		{acts: d("editor"), desc: "open the current file at the cursor line in Neovim listening on /tmp/nvim.<pr node id>.sock and switch to the tmux window \"code\" (no socket: nothing happens)"},
		{acts: d("browser"), desc: "open the PR in the browser"},
		{acts: d("refresh"), desc: "refresh PR, diff and threads"},
		{acts: [][2]string{{keys.Input, "submit"}, {keys.Input, "cancel"}}, desc: "submit / cancel text entry"},
		{literal: "mouse", desc: "click a line to move the cursor, drag or shift+click to select lines, wheel to scroll; click a file or folder in the file list; click a PR in the list, twice to open"},
		{acts: d("help"), desc: "toggle this help"},
		{acts: d("back"), desc: "back to the pull request list"},
		{acts: d("quit"), desc: "back to the list when opened from it, otherwise quit"},
		{acts: d("force_quit"), literal: m.keys.Label(keys.Diff, "force_quit") + " / ctrl+c", desc: "quit"},
	}
	where := "built-in defaults"
	if m.keysPath != "" {
		where = m.keysPath
	}
	lines := []string{
		padRight(styTitle.Render(" Keys")+styDim.Render("  bindings: "+where+" · ghpr --print-keys prints the defaults to edit"), width),
		styBorder.Render(strings.Repeat("─", width)),
	}
	for _, r := range rows {
		label := r.literal
		if label == "" {
			parts := make([]string, len(r.acts))
			for i, a := range r.acts {
				parts[i] = m.keys.Label(a[0], a[1])
			}
			label = strings.Join(parts, " / ")
		}
		lines = append(lines, padRight("  "+styHelpKey.Render(padRight(label, 30))+styDim.Render(r.desc), width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}
