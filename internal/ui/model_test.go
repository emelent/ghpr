package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
	"ghpr/internal/state"
)

const sample = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,6 +1,8 @@
 package main
 
-import "fmt"
+import (
+	"fmt"
+)
 
 func main() {
 	fmt.Println("hi") // multi
diff --git a/b.py b/b.py
--- a/b.py
+++ b/b.py
@@ -1,2 +1,2 @@
 def f():
-    return 1
+    return 2
`

func newTestModel(t *testing.T) *Model {
	t.Helper()
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.pr = &gh.PR{Number: 7, Title: "Test PR", State: "OPEN", HeadRefName: "feat", BaseRefName: "main", HeadRefOid: "abc"}
	m.pending = 2
	m.Update(diffMsg{files: diff.Parse(sample)})
	threads := []gh.Thread{
		{ID: "T1", Path: "main.go", Line: 4, DiffSide: "RIGHT", Comments: []gh.Comment{
			{DatabaseID: 1, Author: "alice", Body: "Why the parens?\nSecond line that is fairly long and should wrap somewhere around the pane width hopefully yes.", CreatedAt: time.Now()},
			{DatabaseID: 2, Author: "bob", Body: "Style.", CreatedAt: time.Now()},
		}},
		{ID: "T2", Path: "main.go", Line: 3, DiffSide: "LEFT", IsResolved: true, Comments: []gh.Comment{{DatabaseID: 3, Author: "carol", Body: "old import", CreatedAt: time.Now()}}},
		{ID: "T3", Path: "b.py", Line: 0, OriginalLine: 40, IsOutdated: true, DiffSide: "RIGHT", Comments: []gh.Comment{{DatabaseID: 4, Author: "dan", Body: "gone", CreatedAt: time.Now()}}},
	}
	m.Update(threadsMsg{threads: threads})
	return m
}

func lines(s string) []string { return strings.Split(s, "\n") }

func TestViewDimensions(t *testing.T) {
	m := newTestModel(t)
	for _, split := range []bool{false, true} {
		m.split = split
		m.rebuildRows()
		out := lines(m.View().Content)
		if len(out) != 30 {
			t.Fatalf("split=%v want 30 lines, got %d", split, len(out))
		}
		for i, l := range out {
			if w := ansi.StringWidth(l); w != 120 {
				t.Fatalf("split=%v line %d width %d: %q", split, i, w, ansi.Strip(l))
			}
		}
	}
}

func TestThreadsInterleaved(t *testing.T) {
	m := newTestModel(t)
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "@alice") || !strings.Contains(plain, "↳ @bob") {
		t.Fatalf("thread not rendered:\n%s", plain)
	}
	// Thread on LEFT:3 (the removed import) appears right after the del line.
	var kinds []rowKind
	for _, r := range m.rows {
		kinds = append(kinds, r.kind)
	}
	// hunk, ctx, ctx, del, thread(T2), add, add, thread(T1), add, ctx, ctx, ctx
	want := []rowKind{rowHunk, rowLine, rowLine, rowLine, rowThread, rowLine, rowLine, rowThread, rowLine, rowLine, rowLine, rowLine}
	if len(kinds) != len(want) {
		t.Fatalf("rows %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("row %d: got %v want %v (%v)", i, kinds[i], want[i], kinds)
		}
	}
	if m.rows[4].thread.ID != "T2" || m.rows[7].thread.ID != "T1" {
		t.Fatalf("thread order wrong")
	}
}

func TestOutdatedThreadAtEnd(t *testing.T) {
	m := newTestModel(t)
	m.selectFile(1)
	last := m.rows[len(m.rows)-1]
	if last.kind != rowThread || last.thread.ID != "T3" {
		t.Fatalf("outdated thread should be last: %+v", last)
	}
	if m.rows[len(m.rows)-2].kind != rowNote {
		t.Fatalf("expected note before outdated thread")
	}
}

func TestAnchors(t *testing.T) {
	m := newTestModel(t)
	// row 3 is the deleted import -> LEFT 3
	line, side, ok := m.rows[3].anchor()
	if !ok || side != "LEFT" || line != 3 {
		t.Fatalf("del anchor %d %s %v", line, side, ok)
	}
	line, side, ok = m.rows[5].anchor()
	if !ok || side != "RIGHT" || line != 3 {
		t.Fatalf("add anchor %d %s %v", line, side, ok)
	}
	if _, _, ok := m.rows[0].anchor(); ok {
		t.Fatalf("hunk header must not be commentable")
	}
	m.split = true
	m.rebuildRows()
	for _, r := range m.rows {
		if r.kind == rowSplit && r.left != nil && r.right != nil && r.left.Kind == diff.Del {
			line, side, _ := r.anchor()
			if side != "RIGHT" || line != r.right.NewNum {
				t.Fatalf("split anchor prefers right side")
			}
		}
	}
}

func TestJumpThreadAcrossFiles(t *testing.T) {
	m := newTestModel(t)
	m.jumpThread(1)
	if m.rows[m.cursor].kind != rowThread || m.rows[m.cursor].thread.ID != "T2" {
		t.Fatalf("first jump -> T2, got %v", m.cursor)
	}
	m.jumpThread(1)
	if m.rows[m.cursor].thread.ID != "T1" {
		t.Fatalf("second jump -> T1")
	}
	m.jumpThread(1)
	if m.fileIdx != 1 || m.rows[m.cursor].thread.ID != "T3" {
		t.Fatalf("third jump should cross into b.py, fileIdx=%d", m.fileIdx)
	}
	m.jumpThread(1)
	if m.fileIdx != 0 || m.rows[m.cursor].thread.ID != "T2" {
		t.Fatalf("wraps around to T2")
	}
}

func TestInputFlow(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 5 // add line -> RIGHT 3
	m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.overlay != overlayInput || m.inComment.Side != "RIGHT" || m.inComment.Line != 3 || m.inComment.Path != "main.go" {
		t.Fatalf("comment input not opened: %+v", m.overlay)
	}
	out := lines(m.View().Content)
	if len(out) != 30 {
		t.Fatalf("with input want 30 lines got %d", len(out))
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "New comment on main.go:3 (RIGHT)") {
		t.Fatalf("title missing")
	}
	// Empty submit is rejected.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModSuper})
	if m.overlay != overlayInput || !m.statusErr {
		t.Fatalf("empty body should be rejected")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != overlayNone {
		t.Fatalf("esc should close")
	}
	// Reply requires a thread row.
	m.cursor = 4
	m.handleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.overlay != overlayInput || m.inThread == nil || m.inThread.ID != "T2" {
		t.Fatalf("reply not opened")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	// Review menu.
	m.handleKey(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if m.overlay != overlayReview {
		t.Fatalf("review menu")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.overlay != overlayInput || m.inEvent != gh.Approve {
		t.Fatalf("approve input")
	}
}

func TestNarrowAndHelp(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	m.split = true
	m.rebuildRows()
	for _, l := range lines(m.View().Content) {
		if ansi.StringWidth(l) != 60 {
			t.Fatalf("narrow width mismatch: %q", ansi.Strip(l))
		}
	}
	m.overlay = overlayHelp
	if len(lines(m.View().Content)) != 15 {
		t.Fatalf("help height")
	}
	m.showFiles = false
	m.overlay = overlayNone
	m.invalidateLayout()
	for _, l := range lines(m.View().Content) {
		if ansi.StringWidth(l) != 60 {
			t.Fatalf("no-files width mismatch: %q", ansi.Strip(l))
		}
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	m.cursor = len(m.rows) - 1
	m.View()
	if m.scroll == 0 {
		t.Fatalf("expected scroll to move to keep cursor visible")
	}
	m.cursor = 0
	m.View()
	if m.scroll != 0 {
		t.Fatalf("expected scroll back to top")
	}
}

func TestRangeSelectionComment(t *testing.T) {
	m := newTestModel(t)
	// rows: 0 hunk, 1 ctx(1), 2 ctx(2), 3 del(old 3), 4 thread, 5 add(new 3), 6 add(new 4), 7 thread, 8 add(new 5) ...
	m.cursor = 5
	m.handleKey(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if !m.selecting || m.selAnchor != 5 {
		t.Fatalf("selection not started")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"}) // now on row 8; row 7 is a thread and is skipped
	if got := m.selectedLineCount(); got != 3 {
		t.Fatalf("selected lines = %d, want 3", got)
	}
	if !m.inSelection(6) || m.inSelection(4) {
		t.Fatalf("inSelection wrong")
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "3 line(s) selected") {
		t.Fatalf("status bar should show selection:\n%s", plain)
	}
	m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	lc := m.inComment
	if m.overlay != overlayInput || lc.StartLine != 3 || lc.Line != 5 || lc.Side != "RIGHT" || lc.StartSide != "RIGHT" || !lc.IsRange() {
		t.Fatalf("range comment anchor wrong: %+v", lc)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "main.go:3-5 (RIGHT)") {
		t.Fatalf("title should show the range")
	}
	if !m.selecting {
		t.Fatalf("selection should stay highlighted while typing")
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.selecting {
		t.Fatalf("cancel should clear the selection")
	}

	// Mixed deletion + addition: each end keeps its own side.
	m.cursor = 3
	m.toggleSelection()
	m.cursor = 5
	start, end, ss, es, ok := rangeAnchor(m.rows, m.selAnchor, m.cursor)
	if !ok || start != 3 || ss != "LEFT" || end != 3 || es != "RIGHT" {
		t.Fatalf("mixed range: %d %s -> %d %s ok=%v", start, ss, end, es, ok)
	}
	// Selecting upwards works the same as downwards.
	m.clearSelection()
	m.cursor = 6
	m.toggleSelection()
	m.cursor = 5
	start, end, _, _, _ = rangeAnchor(m.rows, m.selAnchor, m.cursor)
	if start != 3 || end != 4 {
		t.Fatalf("upward range: %d-%d", start, end)
	}
	// A single-line selection degrades to a plain comment.
	m.clearSelection()
	m.cursor = 5
	m.toggleSelection()
	m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.inComment.IsRange() || m.inComment.Line != 3 {
		t.Fatalf("single-line selection should not be a range: %+v", m.inComment)
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	// Switching file clears any selection.
	m.toggleSelection()
	m.selectFile(1)
	if m.selecting {
		t.Fatalf("file switch should clear selection")
	}
}

func TestReplySubmitDoesNotPanic(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 4 // thread T2
	m.handleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.overlay != overlayInput || m.inThread == nil {
		t.Fatalf("reply input not opened")
	}
	m.ta.SetValue("looks fine")
	cmd := m.submitInput() // must not dereference the cleared thread pointer
	if cmd == nil {
		t.Fatalf("expected an action command")
	}
	if m.overlay != overlayNone || m.busy == "" {
		t.Fatalf("input should close and a loader should show; overlay=%v busy=%q", m.overlay, m.busy)
	}
}

func TestApplyTheme(t *testing.T) {
	t.Cleanup(func() { _ = ApplyTheme(DefaultTheme) })
	if err := ApplyTheme("nope"); err == nil {
		t.Fatal("unknown theme should error")
	}
	if err := ApplyTheme("solarized-dark"); err != nil {
		t.Fatal(err)
	}
	if colAppBg != lipgloss.Color("#002b36") || styTitle.GetForeground() != lipgloss.Color("#eee8d5") {
		t.Fatalf("solarized palette not applied")
	}
	m := newTestModel(t)
	if !strings.Contains(m.View().Content, "\x1b[") {
		t.Fatalf("themed view should carry colour")
	}
	if got := ThemeNames(); len(got) < 2 || got[0] != "github-dark" {
		t.Fatalf("theme names: %v", got)
	}
}

func TestJumpChange(t *testing.T) {
	m := newTestModel(t)
	// rows: 0 hunk, 1 ctx, 2 ctx, 3 del, 4 thread, 5 add, 6 add, 7 thread, 8 add, 9 ctx, 10 ctx, 11 ctx
	m.cursor = 0
	m.jumpChange(1)
	if m.cursor != 3 {
		t.Fatalf("first change at row 3, got %d", m.cursor)
	}
	m.jumpChange(1)
	if m.cursor != 5 {
		t.Fatalf("next block starts at row 5 (thread row splits blocks), got %d", m.cursor)
	}
	m.jumpChange(1)
	if m.cursor != 8 {
		t.Fatalf("next block at row 8, got %d", m.cursor)
	}
	m.jumpChange(1)
	if m.cursor != 8 {
		t.Fatalf("no further change: cursor stays, got %d", m.cursor)
	}
	m.cursor = 6 // middle of the 5-6 block
	m.jumpChange(-1)
	if m.cursor != 5 {
		t.Fatalf("prev from inside a block goes to its start, got %d", m.cursor)
	}
	m.jumpChange(-1)
	if m.cursor != 3 {
		t.Fatalf("prev block at row 3, got %d", m.cursor)
	}
	m.jumpChange(-1)
	if m.cursor != 3 {
		t.Fatalf("no earlier change: cursor stays, got %d", m.cursor)
	}
}

func TestFullFileView(t *testing.T) {
	m := newTestModel(t)
	if !fullEligible(&m.files[0]) {
		t.Fatalf("modified file should be eligible")
	}
	m.cursor = 6 // add line new=4
	m.handleKey(tea.KeyPressMsg{Code: 'F', Text: "F"})
	if !m.full || !m.fullPending[0] || m.busy == "" {
		t.Fatalf("F should request the file and show a loader: full=%v pending=%v busy=%q", m.full, m.fullPending, m.busy)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "loading full file") {
		t.Fatalf("header should say loading")
	}
	content := "package main\n\nimport (\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(\"hi\") // multi\n\tfmt.Println(\"extra line outside the diff\")\n}\n"
	m.Update(fileContentMsg{idx: 0, path: "main.go", text: content})
	if !m.showingFull() || m.busy != "" {
		t.Fatalf("full view should be active after content arrives")
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "full file") || !strings.Contains(plain, "extra line outside the diff") {
		t.Fatalf("full view missing content:\n%s", plain)
	}
	if strings.Contains(plain, "@@ -1,6") {
		t.Fatalf("hunk headers should be hidden in full view")
	}
	// Cursor stays on the same new-side line.
	if o, n, ok := m.rows[m.cursor].nums(); !ok || n != 4 || o != 0 {
		t.Fatalf("cursor should still be on new line 4, got old=%d new=%d ok=%v", o, n, ok)
	}
	// Threads still interleave under their lines.
	if !strings.Contains(plain, "@alice") {
		t.Fatalf("threads should render in full view")
	}
	// Commenting on a line outside the diff is refused.
	for i := range m.rows {
		if _, n, ok := m.rows[i].nums(); ok && n == 9 {
			m.cursor = i
		}
	}
	m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.overlay == overlayInput || !m.statusErr {
		t.Fatalf("comment outside diff should be refused")
	}
	// Toggle back keeps the line where possible.
	m.cursor = 0
	for i := range m.rows {
		if _, n, ok := m.rows[i].nums(); ok && n == 5 {
			m.cursor = i
			break
		}
	}
	m.handleKey(tea.KeyPressMsg{Code: 'F', Text: "F"})
	if m.full || m.showingFull() {
		t.Fatalf("F again should return to hunks")
	}
	if _, n, _ := m.rows[m.cursor].nums(); n != 5 {
		t.Fatalf("cursor should remain on new line 5, got %d", n)
	}
	// Failed fetch falls back to hunks with an error status.
	m.handleKey(tea.KeyPressMsg{Code: 'F', Text: "F"})
	m.selectFile(1)
	m.fullPending[1] = true
	m.Update(fileContentMsg{idx: 1, path: "b.py", err: errString("boom")})
	if m.showingFull() || !m.fullFailed[1] || !m.statusErr {
		t.Fatalf("failed fetch should fall back")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestViewedMarks(t *testing.T) {
	m := newTestModel(t)
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.SetStore(st)
	// Re-deliver the diff so fingerprints are computed with the store attached.
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(sample)})
	if m.viewedCount() != 0 {
		t.Fatal("nothing viewed yet")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.isViewed("main.go") || m.fileIdx != 1 {
		t.Fatalf("m should mark main.go and advance; viewed=%v idx=%d", m.isViewed("main.go"), m.fileIdx)
	}
	v, _ := m.viewedInfo("main.go")
	if v.Fingerprint == "" || v.HeadSHA != "abc" || time.Since(v.ViewedAt) > time.Minute {
		t.Fatalf("viewed record incomplete: %+v", v)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "1 viewed") {
		t.Fatalf("file panel should show viewed count:\n%s", plain)
	}
	m.selectFile(0)
	if !strings.Contains(ansi.Strip(m.View().Content), "viewed just now") {
		t.Fatalf("header should show when the file was viewed")
	}
	// Persisted on disk.
	st2, _ := state.Open(filepath.Dir(st.Path()))
	if _, ok := st2.Get(state.PRKey("o/r", 7), "main.go"); !ok {
		t.Fatal("mark should be saved to disk")
	}
	// Toggle off.
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.isViewed("main.go") || m.fileIdx != 0 {
		t.Fatal("second m should unmark without advancing")
	}
	// Mark again, then reload a diff where main.go changed: it is unmarked, b.py stays.
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"}) // marks b.py too (last file, no advance)
	if m.viewedCount() != 2 {
		t.Fatalf("expected 2 viewed, got %d", m.viewedCount())
	}
	changed := strings.Replace(sample, "fmt.Println(\"hi\")", "fmt.Println(\"changed\")", 1)
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(changed)})
	if m.isViewed("main.go") || !m.isViewed("b.py") {
		t.Fatalf("main.go should be auto-unmarked, b.py kept: main=%v b=%v", m.isViewed("main.go"), m.isViewed("b.py"))
	}
	if !strings.Contains(m.status, "1 file(s) changed since you viewed them") {
		t.Fatalf("status should report the reset: %q", m.status)
	}
	// A file that disappears from the PR is dropped as well.
	onlyMain := sample[:strings.Index(sample, "diff --git a/b.py")]
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(onlyMain)})
	if len(st.Paths(state.PRKey("o/r", 7))) != 0 {
		t.Fatal("b.py left the PR and should be dropped")
	}
	// Without a store the key is a no-op with a message.
	m.SetStore(nil)
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.statusErr {
		t.Fatal("expected disabled message")
	}
}
