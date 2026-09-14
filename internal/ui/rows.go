package ui

import (
	"fmt"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
)

type rowKind int

const (
	rowHunk rowKind = iota
	rowLine
	rowSplit
	rowThread
	rowNote
)

// row is a selectable unit in the diff pane.
type row struct {
	kind   rowKind
	hunk   *diff.Hunk
	line   *diff.Line // rowLine
	left   *diff.Line // rowSplit
	right  *diff.Line // rowSplit
	thread *gh.Thread // rowThread
	note   string     // rowNote
}

// anchor returns where a new comment on this row should be placed.
func (r *row) anchor() (line int, side string, ok bool) {
	switch r.kind {
	case rowLine:
		if r.line.Kind == diff.Del {
			return r.line.OldNum, "LEFT", true
		}
		return r.line.NewNum, "RIGHT", true
	case rowSplit:
		if r.right != nil {
			return r.right.NewNum, "RIGHT", true
		}
		if r.left != nil {
			return r.left.OldNum, "LEFT", true
		}
	case rowThread:
		if r.thread.Line > 0 {
			return r.thread.Line, r.thread.DiffSide, true
		}
	}
	return 0, "", false
}

// nums returns the old/new line numbers a row covers (0 when absent).
// ok is false for rows that are not diff lines.
func (r *row) nums() (oldNum, newNum int, ok bool) {
	switch r.kind {
	case rowLine:
		return r.line.OldNum, r.line.NewNum, true
	case rowSplit:
		if r.left != nil {
			oldNum = r.left.OldNum
		}
		if r.right != nil {
			newNum = r.right.NewNum
		}
		return oldNum, newNum, r.left != nil || r.right != nil
	}
	return 0, 0, false
}

// rangeAnchor resolves a selection of rows [lo, hi] into a comment anchor.
// It prefers keeping the whole range on one side of the diff; when the
// selection mixes removed and added lines each end uses its own side.
func rangeAnchor(rows []row, lo, hi int) (start, end int, startSide, side string, ok bool) {
	if lo > hi {
		lo, hi = hi, lo
	}
	var lineRows []*row
	allNew, allOld := true, true
	for i := lo; i <= hi && i < len(rows); i++ {
		r := &rows[i]
		o, n, isLine := r.nums()
		if !isLine {
			continue
		}
		lineRows = append(lineRows, r)
		if n == 0 {
			allNew = false
		}
		if o == 0 {
			allOld = false
		}
	}
	if len(lineRows) == 0 {
		return 0, 0, "", "", false
	}
	first, last := lineRows[0], lineRows[len(lineRows)-1]
	switch {
	case allNew:
		_, s, _ := first.nums()
		_, e, _ := last.nums()
		return s, e, "RIGHT", "RIGHT", true
	case allOld:
		s, _, _ := first.nums()
		e, _, _ := last.nums()
		return s, e, "LEFT", "LEFT", true
	}
	s, ss, _ := first.anchor()
	e, es, _ := last.anchor()
	return s, e, ss, es, true
}

type anchorKey struct {
	side string
	line int
}

// buildRows flattens a file into rows, interleaving review threads under the
// lines they are anchored to. Threads that no longer map onto the diff are
// appended at the end of the file.
func buildRows(f *diff.File, split bool, threads []gh.Thread) []row {
	var rows []row
	path := f.Path()

	byAnchor := map[anchorKey][]*gh.Thread{}
	var fileThreads []*gh.Thread
	for i := range threads {
		t := &threads[i]
		if t.Path != path {
			continue
		}
		fileThreads = append(fileThreads, t)
		if t.Line > 0 {
			k := anchorKey{t.DiffSide, t.Line}
			byAnchor[k] = append(byAnchor[k], t)
		}
	}
	shown := map[string]bool{}
	attach := func(l *diff.Line) {
		if l == nil {
			return
		}
		var keys []anchorKey
		if l.Kind != diff.Del && l.NewNum > 0 {
			keys = append(keys, anchorKey{"RIGHT", l.NewNum})
		}
		if l.Kind != diff.Add && l.OldNum > 0 {
			keys = append(keys, anchorKey{"LEFT", l.OldNum})
		}
		for _, k := range keys {
			for _, t := range byAnchor[k] {
				if shown[t.ID] {
					continue
				}
				shown[t.ID] = true
				rows = append(rows, row{kind: rowThread, thread: t})
			}
		}
	}

	switch {
	case f.IsBinary:
		rows = append(rows, row{kind: rowNote, note: "Binary file – contents not shown"})
	case f.PatchOmitted && len(f.Hunks) == 0:
		rows = append(rows, row{kind: rowNote, note: fmt.Sprintf("GitHub left this patch out of the API (+%d -%d, too large) – press F to load the full file", f.Additions, f.Deletions)})
	case len(f.Hunks) == 0:
		rows = append(rows, row{kind: rowNote, note: "No textual changes (rename, mode change or empty file)"})
	}

	for hi := range f.Hunks {
		h := &f.Hunks[hi]
		if !f.Full {
			rows = append(rows, row{kind: rowHunk, hunk: h})
		}
		if split {
			for _, p := range diff.SideBySide(h) {
				rows = append(rows, row{kind: rowSplit, left: p.Left, right: p.Right})
				attach(p.Left)
				if p.Right != p.Left {
					attach(p.Right)
				}
			}
		} else {
			for li := range h.Lines {
				l := &h.Lines[li]
				rows = append(rows, row{kind: rowLine, line: l})
				attach(l)
			}
		}
	}

	var rest []*gh.Thread
	for _, t := range fileThreads {
		if !shown[t.ID] {
			rest = append(rest, t)
		}
	}
	if len(rest) > 0 {
		rows = append(rows, row{kind: rowNote, note: fmt.Sprintf("%d thread(s) on lines outside this diff (outdated)", len(rest))})
		for _, t := range rest {
			rows = append(rows, row{kind: rowThread, thread: t})
		}
	}
	return rows
}
