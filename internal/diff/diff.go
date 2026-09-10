// Package diff parses unified diffs (as produced by `gh pr diff`) into a
// structured form suitable for rendering inline or side by side.
package diff

import (
	"regexp"
	"strconv"
	"strings"
)

// Kind classifies a diff line.
type Kind int

const (
	Context Kind = iota
	Add
	Del
)

// Line is a single line within a hunk.
type Line struct {
	Kind   Kind
	OldNum int // 0 when the line does not exist on the old side
	NewNum int // 0 when the line does not exist on the new side
	Text   string
}

// Hunk is a contiguous change region.
type Hunk struct {
	Header   string // full "@@ ... @@ ..." line
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Line
}

// Status describes what happened to a file.
type Status string

const (
	Modified Status = "modified"
	Added    Status = "added"
	Deleted  Status = "deleted"
	Renamed  Status = "renamed"
)

// File is one file entry in the diff.
type File struct {
	OldPath   string
	NewPath   string
	Status    Status
	IsBinary  bool
	Hunks     []Hunk
	Additions int
	Deletions int
	// Full is set when the file was expanded with Expand and its single hunk
	// covers the entire file; hunk headers are then meaningless.
	Full bool
}

// Path returns the path to use for display and comment anchoring.
func (f *File) Path() string {
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	return f.OldPath
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
var diffGitRe = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)

// Parse converts a unified diff into files.
func Parse(text string) []File {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var files []File
	var cur *File
	var hunk *Hunk
	oldN, newN := 0, 0

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}

	for _, raw := range lines {
		if m := diffGitRe.FindStringSubmatch(raw); m != nil {
			flushHunk()
			files = append(files, File{OldPath: m[1], NewPath: m[2], Status: Modified})
			cur = &files[len(files)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if hunk == nil {
			// File header section.
			switch {
			case strings.HasPrefix(raw, "new file mode"):
				cur.Status = Added
			case strings.HasPrefix(raw, "deleted file mode"):
				cur.Status = Deleted
			case strings.HasPrefix(raw, "rename from "):
				cur.Status = Renamed
				cur.OldPath = strings.TrimPrefix(raw, "rename from ")
			case strings.HasPrefix(raw, "rename to "):
				cur.Status = Renamed
				cur.NewPath = strings.TrimPrefix(raw, "rename to ")
			case strings.HasPrefix(raw, "Binary files"), strings.HasPrefix(raw, "GIT binary patch"):
				cur.IsBinary = true
			case strings.HasPrefix(raw, "--- "):
				p := strings.TrimPrefix(raw, "--- ")
				if p == "/dev/null" {
					cur.Status = Added
				}
			case strings.HasPrefix(raw, "+++ "):
				p := strings.TrimPrefix(raw, "+++ ")
				if p == "/dev/null" {
					cur.Status = Deleted
				}
			}
		}
		if m := hunkRe.FindStringSubmatch(raw); m != nil {
			flushHunk()
			h := Hunk{Header: raw}
			h.OldStart, _ = strconv.Atoi(m[1])
			h.OldCount = 1
			if m[2] != "" {
				h.OldCount, _ = strconv.Atoi(m[2])
			}
			h.NewStart, _ = strconv.Atoi(m[3])
			h.NewCount = 1
			if m[4] != "" {
				h.NewCount, _ = strconv.Atoi(m[4])
			}
			hunk = &h
			oldN, newN = h.OldStart, h.NewStart
			continue
		}
		if hunk == nil {
			continue
		}
		if raw == "" {
			// Trailing empty line at end of diff; a genuine empty context
			// line is " " (a single space), so this is safe to ignore.
			continue
		}
		switch raw[0] {
		case ' ':
			hunk.Lines = append(hunk.Lines, Line{Kind: Context, OldNum: oldN, NewNum: newN, Text: raw[1:]})
			oldN++
			newN++
		case '+':
			hunk.Lines = append(hunk.Lines, Line{Kind: Add, NewNum: newN, Text: raw[1:]})
			newN++
			cur.Additions++
		case '-':
			hunk.Lines = append(hunk.Lines, Line{Kind: Del, OldNum: oldN, Text: raw[1:]})
			oldN++
			cur.Deletions++
		case '\\':
			// "\ No newline at end of file" – ignore.
		default:
			// Unknown line inside a hunk; treat as context to stay aligned.
			hunk.Lines = append(hunk.Lines, Line{Kind: Context, OldNum: oldN, NewNum: newN, Text: raw})
			oldN++
			newN++
		}
	}
	flushHunk()
	return files
}

// Pair is one row of a side-by-side view. Either side may be nil.
type Pair struct {
	Left  *Line
	Right *Line
}

// SideBySide pairs a hunk's lines into rows: context lines occupy both
// columns, and runs of removals/additions are matched up index-wise.
func SideBySide(h *Hunk) []Pair {
	var rows []Pair
	lines := h.Lines
	for i := 0; i < len(lines); {
		if lines[i].Kind == Context {
			rows = append(rows, Pair{Left: &lines[i], Right: &lines[i]})
			i++
			continue
		}
		var dels, adds []*Line
		for i < len(lines) && lines[i].Kind != Context {
			if lines[i].Kind == Del {
				dels = append(dels, &lines[i])
			} else {
				adds = append(adds, &lines[i])
			}
			i++
		}
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for k := 0; k < n; k++ {
			var p Pair
			if k < len(dels) {
				p.Left = dels[k]
			}
			if k < len(adds) {
				p.Right = adds[k]
			}
			rows = append(rows, p)
		}
	}
	return rows
}

// hunkEnds returns the first old/new line numbers after a hunk. Empty ranges
// in unified diffs report the line *before* the position, so they are
// nudged forward by one.
func hunkEnds(h *Hunk) (oldEnd, newEnd int) {
	oldEnd = h.OldStart + h.OldCount
	if h.OldCount == 0 {
		oldEnd = h.OldStart + 1
	}
	newEnd = h.NewStart + h.NewCount
	if h.NewCount == 0 {
		newEnd = h.NewStart + 1
	}
	return oldEnd, newEnd
}

// Expand merges the full new-side content of a file with its diff hunks,
// producing a File whose single hunk lists every line of the file with the
// hunks' additions and deletions in place. Lines outside the hunks become
// context lines with both old and new numbers.
func Expand(f *File, newContent string) *File {
	newContent = strings.ReplaceAll(newContent, "\r\n", "\n")
	newContent = strings.TrimSuffix(newContent, "\n")
	var lines []string
	if newContent != "" {
		lines = strings.Split(newContent, "\n")
	}
	out := &File{OldPath: f.OldPath, NewPath: f.NewPath, Status: f.Status, IsBinary: f.IsBinary,
		Additions: f.Additions, Deletions: f.Deletions, Full: true}
	h := Hunk{OldStart: 1, NewStart: 1}
	newN, offset, hi := 1, 0, 0 // old = new + offset
	for hi < len(f.Hunks) || newN <= len(lines) {
		if hi < len(f.Hunks) {
			hk := &f.Hunks[hi]
			pos := hk.NewStart
			if hk.NewCount == 0 {
				pos++
			}
			if newN >= pos || newN > len(lines) {
				h.Lines = append(h.Lines, hk.Lines...)
				oldEnd, newEnd := hunkEnds(hk)
				if newEnd > newN {
					newN = newEnd
				}
				offset = oldEnd - newEnd
				hi++
				continue
			}
		}
		h.Lines = append(h.Lines, Line{Kind: Context, OldNum: newN + offset, NewNum: newN, Text: lines[newN-1]})
		newN++
	}
	h.NewCount = len(lines)
	h.OldCount = len(lines) + offset
	out.Hunks = []Hunk{h}
	return out
}

// InDiff reports whether a line number on the given side ("LEFT" or
// "RIGHT") appears in the file's hunks, i.e. GitHub will accept a review
// comment on it.
func (f *File) InDiff(side string, n int) bool {
	for hi := range f.Hunks {
		for _, l := range f.Hunks[hi].Lines {
			if side == "LEFT" && l.OldNum == n && n > 0 {
				return true
			}
			if side == "RIGHT" && l.NewNum == n && n > 0 {
				return true
			}
		}
	}
	return false
}
