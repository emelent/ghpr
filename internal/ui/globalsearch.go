package ui

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---------- search across all files (ctrl+/) ----------

// gsMaxHits caps the result list so huge diffs stay responsive.
const gsMaxHits = 500

// gsHit is one matching diff line somewhere in the PR.
type gsHit struct {
	fileIdx        int
	oldNum, newNum int      // to find the row again, whatever the view mode
	text           string   // tab-expanded line text
	ranges         [][2]int // matched rune ranges in text
	exact          bool     // the query occurs verbatim (else a fuzzy hit)
}

// globalMatch matches q against a line for the all-files search: verbatim
// occurrences when there are any (exact), otherwise q as an in-order
// subsequence of characters (fuzzy). Case is ignored; spaces in q are
// skipped in fuzzy mode so "fmt print" finds fmt.Println. nil means no match.
func globalMatch(text, q string) (ranges [][2]int, exact bool) {
	if r := findMatches(text, q); len(r) > 0 {
		return r, true
	}
	return mergeRanges(fuzzyPositions(text, q)), false
}

// fuzzyPositions returns the rune indexes in text that match q as an
// in-order subsequence, taking the leftmost candidate each time, ignoring
// case; nil when q cannot be found that way.
func fuzzyPositions(text, q string) []int {
	qr := []rune(strings.ReplaceAll(q, " ", ""))
	if len(qr) == 0 {
		return nil
	}
	pos := make([]int, 0, len(qr))
	qi := 0
	for i, t := range []rune(text) {
		if qi == len(qr) {
			break
		}
		if t == qr[qi] || unicode.ToLower(t) == unicode.ToLower(qr[qi]) {
			pos = append(pos, i)
			qi++
		}
	}
	if qi < len(qr) {
		return nil
	}
	return pos
}

// mergeRanges turns sorted rune positions into [start, end) ranges, joining
// adjacent positions.
func mergeRanges(pos []int) [][2]int {
	var out [][2]int
	for _, p := range pos {
		if n := len(out); n > 0 && out[n-1][1] == p {
			out[n-1][1] = p + 1
			continue
		}
		out = append(out, [2]int{p, p + 1})
	}
	return out
}

// openGlobalSearch opens the all-files search prompt.
func (m *Model) openGlobalSearch() {
	if len(m.files) == 0 {
		return
	}
	m.overlay = overlayGlobal
	m.gsQuery = ""
	m.gsResults = nil
	m.gsIdx, m.gsScroll = 0, 0
}

// refilterGlobal recomputes the hits for the current query over every
// diff line of every file: exact hits first, then fuzzy ones, each group in
// file order, capped at gsMaxHits.
func (m *Model) refilterGlobal() {
	m.gsResults = m.gsResults[:0]
	if m.gsQuery == "" {
		return
	}
	var fuzzy []gsHit
	for fi := range m.files {
		f := &m.files[fi]
		for hi := range f.Hunks {
			for li := range f.Hunks[hi].Lines {
				l := &f.Hunks[hi].Lines[li]
				text := expandTabs(l.Text)
				r, exact := globalMatch(text, m.gsQuery)
				if len(r) == 0 {
					continue
				}
				h := gsHit{fileIdx: fi, oldNum: l.OldNum, newNum: l.NewNum, text: text, ranges: r, exact: exact}
				if exact {
					m.gsResults = append(m.gsResults, h)
					if len(m.gsResults) >= gsMaxHits {
						return
					}
				} else if len(fuzzy) < gsMaxHits {
					fuzzy = append(fuzzy, h)
				}
			}
		}
	}
	room := gsMaxHits - len(m.gsResults)
	m.gsResults = append(m.gsResults, fuzzy[:min(room, len(fuzzy))]...)
}

// handleGlobalSearchKey edits the query and moves through the hits.
func (m *Model) handleGlobalSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+/", "ctrl+_":
		m.overlay = overlayNone
		return m, nil
	case "enter":
		return m, m.jumpToHit()
	case "down", "ctrl+n", "ctrl+j", "tab":
		if m.gsIdx+1 < len(m.gsResults) {
			m.gsIdx++
		}
		return m, nil
	case "up", "ctrl+k", "shift+tab":
		if m.gsIdx > 0 {
			m.gsIdx--
		}
		return m, nil
	case "backspace":
		if r := []rune(m.gsQuery); len(r) > 0 {
			m.gsQuery = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.gsQuery = ""
	default:
		if msg.Text == "" {
			return m, nil // control key: ignore
		}
		m.gsQuery += msg.Text
	}
	m.gsIdx, m.gsScroll = 0, 0
	m.refilterGlobal()
	return m, nil
}

// jumpToHit opens the selected hit's file and puts the cursor on its line.
// For an exact hit the query becomes the active in-file search so n/N and
// highlights carry on; the in-file search is not fuzzy, so a fuzzy hit
// leaves it alone.
func (m *Model) jumpToHit() tea.Cmd {
	if len(m.gsResults) == 0 {
		return nil
	}
	h := m.gsResults[m.gsIdx]
	m.overlay = overlayNone
	m.filesFocused = false
	if h.exact {
		m.searchQ = m.gsQuery
	}
	cmd := m.selectFile(h.fileIdx)
	for i := range m.rows {
		o, n, ok := m.rows[i].nums()
		if ok && (h.newNum > 0 && n == h.newNum || h.newNum == 0 && o == h.oldNum) {
			m.cursor = i
			break
		}
	}
	return cmd
}

// renderGlobalSearch draws the prompt and hits in place of the diff pane.
func (m *Model) renderGlobalSearch(width, height int) []string {
	count := fmt.Sprintf("  %d match(es)", len(m.gsResults))
	if len(m.gsResults) >= gsMaxHits {
		count = fmt.Sprintf("  first %d matches", gsMaxHits)
	}
	title := styTitle.Render(" Search all files") + styDim.Render(count)
	lines := []string{
		padRight(truncate(title, width), width),
		styBorder.Render(strings.Repeat("─", width)),
		padRight(truncate(styAccent.Render(" > ")+m.gsQuery+styAccent.Render("▏"), width), width),
	}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	switch {
	case m.gsQuery == "":
		lines = append(lines, padRight(styNote.Render("   Type to search every file in the diff"), width))
	case len(m.gsResults) == 0:
		lines = append(lines, padRight(styNote.Render("   No matches"), width))
	}
	if m.gsIdx < m.gsScroll {
		m.gsScroll = m.gsIdx
	}
	if m.gsIdx >= m.gsScroll+avail {
		m.gsScroll = m.gsIdx - avail + 1
	}
	for i := m.gsScroll; i < len(m.gsResults) && len(lines) < height; i++ {
		lines = append(lines, m.renderHitRow(&m.gsResults[i], i == m.gsIdx, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// renderHitRow renders "path:line  text" with the matched characters in
// the search highlight; the selected hit sits on the selection background.
func (m *Model) renderHitRow(h *gsHit, selected bool, width int) string {
	bg := color.Color(lipgloss.NoColor{})
	if selected {
		bg = colSelBg
	}
	base := lipgloss.NewStyle().Background(bg)
	num := h.newNum
	sign := ""
	if num == 0 {
		num, sign = h.oldNum, "-"
	}
	loc := fmt.Sprintf(" %s:%s%d ", m.files[h.fileIdx].Path(), sign, num)
	locW := min(ansi.StringWidth(loc), max(8, width/2))
	prefix := base.Foreground(colDim).Render(leftEllipsis(loc, locW))
	textW := width - locW
	if textW < 1 {
		return padRightBg(truncate(prefix, width), width, bg)
	}
	spans := markRanges([]Span{{Text: strings.TrimLeft(h.text, " ")}}, shiftRanges(h.ranges, len(h.text)-len(strings.TrimLeft(h.text, " "))))
	return prefix + renderSpans(spans, bg, textW, 0)
}

// shiftRanges moves rune ranges left by n (dropping what falls below 0),
// for text whose leading n characters were trimmed.
func shiftRanges(ranges [][2]int, n int) [][2]int {
	if n == 0 {
		return ranges
	}
	out := make([][2]int, 0, len(ranges))
	for _, r := range ranges {
		s, e := r[0]-n, r[1]-n
		if e <= 0 {
			continue
		}
		out = append(out, [2]int{max(0, s), e})
	}
	return out
}
