package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"ghpr/internal/diff"
)

// ---------- in-file search (/) ----------

// openSearch opens the / prompt. Typing searches incrementally from the row
// the cursor is on; enter keeps the match, esc goes back to where we were.
func (m *Model) openSearch() {
	m.overlay = overlaySearch
	m.searchPrev = m.searchQ
	m.searchInput = ""
	m.searchFrom = m.cursor
}

// handleSearchKey edits the / prompt.
func (m *Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		m.searchQ = m.searchPrev
		m.cursor = m.searchFrom
		return m, nil
	case "enter":
		m.overlay = overlayNone
		m.searchQ = m.searchInput
		if m.searchQ == "" {
			return m, nil
		}
		return m, m.searchJump(m.searchFrom, 1, true)
	case "backspace":
		if r := []rune(m.searchInput); len(r) > 0 {
			m.searchInput = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.searchInput = ""
	default:
		if msg.Text == "" {
			return m, nil // control key: ignore
		}
		m.searchInput += msg.Text
	}
	// Incremental preview: highlight and jump from the origin row.
	m.searchQ = m.searchInput
	m.cursor = m.searchFrom
	if i := m.findMatch(m.searchFrom, 1, true); i >= 0 {
		m.cursor = i
	}
	return m, nil
}

// searchStep moves to the next (dir > 0) or previous match of the active
// search, wrapping around the file.
func (m *Model) searchStep(dir int) tea.Cmd { return m.searchJump(m.cursor, dir, false) }

// searchJump moves the cursor to the nearest match from row from in
// direction dir, wrapping around; inclusive allows from itself.
func (m *Model) searchJump(from, dir int, inclusive bool) tea.Cmd {
	if m.searchQ == "" {
		return nil
	}
	i := m.findMatch(from, dir, inclusive)
	if i < 0 {
		return m.setStatus(fmt.Sprintf("No match for “%s” in this file", m.searchQ), true)
	}
	m.cursor = i
	return nil
}

// findMatch returns the index of the nearest matching row from row from in
// direction dir, wrapping around the file, or -1 when nothing matches.
func (m *Model) findMatch(from, dir int, inclusive bool) int {
	n := len(m.rows)
	if n == 0 || m.searchQ == "" {
		return -1
	}
	start := from
	if !inclusive {
		start += dir
	}
	for k := 0; k < n; k++ {
		i := ((start+dir*k)%n + n) % n
		if m.rowMatches(i) {
			return i
		}
	}
	return -1
}

// rowMatches reports whether the diff text on row i contains the query.
func (m *Model) rowMatches(i int) bool {
	if i < 0 || i >= len(m.rows) || m.searchQ == "" {
		return false
	}
	has := func(l *diff.Line) bool { return l != nil && len(findMatches(l.Text, m.searchQ)) > 0 }
	r := &m.rows[i]
	switch r.kind {
	case rowLine:
		return has(r.line)
	case rowSplit:
		return has(r.left) || has(r.right)
	}
	return false
}

// matchPos returns the 1-based position of the cursor row among the matching
// rows (0 when the cursor is not on a match) and the total number of matches.
func (m *Model) matchPos() (pos, total int) {
	for i := range m.rows {
		if m.rowMatches(i) {
			total++
			if i == m.cursor {
				pos = total
			}
		}
	}
	return pos, total
}

// lineSpans returns the syntax spans for a line with search matches marked.
func (m *Model) lineSpans(l *diff.Line) []Span {
	sp := m.spans[l]
	if m.searchQ == "" {
		return sp
	}
	return markSpans(sp, m.searchQ)
}

// findMatches returns the [start, end) rune ranges where q occurs in text.
// Matching ignores case unless q contains an upper-case letter (smart case).
func findMatches(text, q string) [][2]int {
	if q == "" {
		return nil
	}
	tr, qr := []rune(text), []rune(q)
	fold := !strings.ContainsFunc(q, unicode.IsUpper)
	var out [][2]int
	for i := 0; i+len(qr) <= len(tr); {
		if runesMatchAt(tr, qr, i, fold) {
			out = append(out, [2]int{i, i + len(qr)})
			i += len(qr)
			continue
		}
		i++
	}
	return out
}

func runesMatchAt(text, q []rune, at int, fold bool) bool {
	for j, r := range q {
		t := text[at+j]
		if t == r {
			continue
		}
		if fold && unicode.ToLower(t) == unicode.ToLower(r) {
			continue
		}
		return false
	}
	return true
}

// markSpans splits spans so every occurrence of q is its own span with
// Match set, so it can be drawn with the search highlight.
func markSpans(spans []Span, q string) []Span {
	var sb strings.Builder
	for _, sp := range spans {
		sb.WriteString(sp.Text)
	}
	ranges := findMatches(sb.String(), q)
	if len(ranges) == 0 {
		return spans
	}
	out := make([]Span, 0, len(spans)+2*len(ranges))
	pos, ri := 0, 0 // rune offset of the current span; next range to place
	for _, sp := range spans {
		r := []rune(sp.Text)
		start := 0
		for start < len(r) {
			for ri < len(ranges) && ranges[ri][1] <= pos+start {
				ri++
			}
			if ri >= len(ranges) || ranges[ri][0] >= pos+len(r) {
				rest := sp
				rest.Text = string(r[start:])
				out = append(out, rest)
				break
			}
			ms, me := ranges[ri][0]-pos, ranges[ri][1]-pos
			if ms > start {
				pre := sp
				pre.Text = string(r[start:ms])
				out = append(out, pre)
				start = ms
			}
			end := min(me, len(r))
			hit := sp
			hit.Text, hit.Match = string(r[start:end]), true
			out = append(out, hit)
			start = end
		}
		pos += len(r)
	}
	return out
}
