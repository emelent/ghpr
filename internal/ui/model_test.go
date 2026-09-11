package ui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/diff"
	"ghpr/internal/editor"
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
	m.pr = &gh.PR{ID: "PR_test7", Number: 7, Title: "Test PR", State: "OPEN", HeadRefName: "feat", BaseRefName: "main", HeadRefOid: "abc"}
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
	m.handleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModSuper})
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
	if !m.isViewed("main.go") || m.fileIdx != 0 || !m.filesFocused {
		t.Fatalf("m should mark main.go, stay on it and focus the file panel; viewed=%v idx=%d focused=%v", m.isViewed("main.go"), m.fileIdx, m.filesFocused)
	}
	m.filesFocused = false
	v, _ := m.viewedInfo("main.go")
	if v.Fingerprint == "" || v.HeadSHA != "abc" || time.Since(v.ViewedAt) > time.Minute {
		t.Fatalf("viewed record incomplete: %+v", v)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "1 viewed") {
		t.Fatalf("file panel should show viewed count:\n%s", plain)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "viewed just now") {
		t.Fatalf("header should show when the file was viewed")
	}
	// Persisted on disk.
	st2, _ := state.Open(filepath.Dir(st.Path()))
	if _, ok := st2.Get(state.PRKey("o/r", 7), "main.go"); !ok {
		t.Fatal("mark should be saved to disk")
	}
	// Toggle off (from the diff; unmarking does not change focus).
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if m.isViewed("main.go") || m.fileIdx != 0 || m.filesFocused {
		t.Fatal("second m should unmark and stay in the diff")
	}
	// Mark both files, then reload a diff where main.go changed: it is unmarked, b.py stays.
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m.selectFile(1)
	m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
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

const treeSample = `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-a
+b
diff --git a/internal/ui/model.go b/internal/ui/model.go
--- a/internal/ui/model.go
+++ b/internal/ui/model.go
@@ -1 +1 @@
-a
+b
diff --git a/internal/ui/render.go b/internal/ui/render.go
--- a/internal/ui/render.go
+++ b/internal/ui/render.go
@@ -1 +1,2 @@
-a
+b
+c
diff --git a/cmd/x/deep/main.go b/cmd/x/deep/main.go
--- a/cmd/x/deep/main.go
+++ b/cmd/x/deep/main.go
@@ -1 +1 @@
-a
+b
`

func newTreeModel(t *testing.T) *Model {
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.pr = &gh.PR{Number: 7, HeadRefOid: "abc"}
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(treeSample)})
	return m
}

func labels(nodes []treeNode) []string {
	var out []string
	for _, n := range nodes {
		l := strings.Repeat(" ", n.depth) + n.label
		if n.isDir {
			l += "/"
		}
		out = append(out, l)
	}
	return out
}

func TestTreeBuild(t *testing.T) {
	m := newTreeModel(t)
	got := strings.Join(labels(m.treeNodes), "|")
	want := "cmd/x/deep/| main.go|internal/ui/| model.go| render.go|README.md"
	if got != want {
		t.Fatalf("tree\n got %s\nwant %s", got, want)
	}
	// Aggregates on the compacted node.
	if n := m.treeNodes[2]; n.files != 2 || n.adds != 3 || n.dels != 2 {
		t.Fatalf("aggregate %+v", n)
	}
	m.collapsed["internal/ui"] = true
	m.rebuildTree()
	if got := strings.Join(labels(m.treeNodes), "|"); got != "cmd/x/deep/| main.go|internal/ui/|README.md" {
		t.Fatalf("collapsed tree: %s", got)
	}
}

func TestTreeNavigation(t *testing.T) {
	m := newTreeModel(t)
	key := func(k string) {
		switch k {
		case "enter":
			m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		case "tab":
			m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
		default:
			m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		}
	}
	// File 0 in diff order is README.md; the tree selects it.
	if m.fileIdx != 0 || m.treeSel != "README.md" {
		t.Fatalf("initial selection %d %q", m.fileIdx, m.treeSel)
	}
	key("tab")
	key("k") // render.go
	if m.files[m.fileIdx].Path() != "internal/ui/render.go" {
		t.Fatalf("k should move onto render.go, got %s", m.files[m.fileIdx].Path())
	}
	key("k") // model.go
	key("k") // dir internal/ui: diff stays on model.go
	if m.treeSel != "internal/ui" || m.files[m.fileIdx].Path() != "internal/ui/model.go" {
		t.Fatalf("dir node selected but file kept: %q %s", m.treeSel, m.files[m.fileIdx].Path())
	}
	key("enter") // collapse
	if !m.collapsed["internal/ui"] || len(m.treeNodes) != 4 {
		t.Fatalf("enter should collapse: %v %d", m.collapsed, len(m.treeNodes))
	}
	key("l") // expand
	if m.collapsed["internal/ui"] {
		t.Fatal("l should expand")
	}
	key("h") // collapse again
	if !m.collapsed["internal/ui"] {
		t.Fatal("h should collapse")
	}
	// Selecting a hidden file from the diff side reveals it.
	m.filesFocused = false
	m.selectFile(2) // render.go
	if m.collapsed["internal/ui"] || m.treeSel != "internal/ui/render.go" {
		t.Fatalf("selectFile should reveal: collapsed=%v sel=%q", m.collapsed, m.treeSel)
	}
	// h on a file jumps to its directory.
	m.filesFocused = true
	key("h")
	if m.treeSel != "internal/ui" {
		t.Fatalf("h on file should select parent, got %q", m.treeSel)
	}
	key("H")
	if !m.collapsed["cmd/x/deep"] || m.collapsed["internal/ui"] {
		t.Fatalf("H collapses all but keeps the current file visible: %v", m.collapsed)
	}
	key("L")
	if len(m.collapsed) != 0 {
		t.Fatal("L expands all")
	}
	// Enter on a file returns focus to the diff.
	m.treeSel = "README.md"
	key("enter")
	if m.filesFocused || m.fileIdx != 0 {
		t.Fatal("enter on file should open it and unfocus the panel")
	}
	// Flat mode still works and renders full-width.
	key("t")
	if m.tree {
		t.Fatal("t toggles flat")
	}
	for _, l := range lines(m.View().Content) {
		if ansi.StringWidth(l) != 120 {
			t.Fatalf("flat width %q", ansi.Strip(l))
		}
	}
	key("t")
	for _, l := range lines(m.View().Content) {
		if ansi.StringWidth(l) != 120 {
			t.Fatalf("tree width %q", ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "internal/ui/") {
		t.Fatal("tree should render directory nodes")
	}
}

func TestJumpChangeKeys(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 0
	m.handleKey(tea.KeyPressMsg{Code: 'J', Text: "J", Mod: tea.ModShift})
	if m.cursor != 3 {
		t.Fatalf("J should jump to the first change (row 3), got %d", m.cursor)
	}
	m.handleKey(tea.KeyPressMsg{Code: 'J', Text: "J", Mod: tea.ModShift})
	m.handleKey(tea.KeyPressMsg{Code: 'K', Text: "K", Mod: tea.ModShift})
	if m.cursor != 3 {
		t.Fatalf("K should return to row 3, got %d", m.cursor)
	}
}

func TestPickerOpensWithL(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 0, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(prListMsg{prs: []gh.PRSummary{{Number: 12, Title: "one"}, {Number: 34, Title: "two"}}})
	if m.screen != screenPicker {
		t.Fatal("expected picker")
	}
	m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.screen != screenDiff || m.number != 34 || cmd == nil {
		t.Fatalf("l should open PR #34 and start loading: screen=%v number=%d", m.screen, m.number)
	}
}

func TestMergeMenu(t *testing.T) {
	m := newTestModel(t)
	m.pr.MergeStateStatus = "BLOCKED"
	m.pr.Body = "Fixes the thing.\n\nDetails here."
	key := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "submit":
			msg = tea.KeyPressMsg{Code: 'j', Mod: tea.ModSuper}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	key("M")
	if m.overlay != overlayMerge {
		t.Fatal("M should open the merge menu")
	}
	bar := ansi.Strip(m.View().Content)
	if !strings.Contains(bar, "BLOCKED") || !strings.Contains(bar, "squash") {
		t.Fatalf("menu should warn about merge state and list methods:\n%s", bar)
	}
	if key("y") != nil {
		t.Fatal("y without rebase chosen must not merge")
	}
	// Squash opens the commit message editor prefilled with GitHub's default.
	key("d")
	key("s")
	if m.overlay != overlayInput || m.inKind != inputMerge || m.mergeMethod != gh.Squash || !m.mergeDelete {
		t.Fatalf("s should open the merge editor: overlay=%v kind=%v", m.overlay, m.inKind)
	}
	if got := m.ta.Value(); got != "Test PR (#7)\n\nFixes the thing.\n\nDetails here." {
		t.Fatalf("prefilled message %q", got)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "edit the commit message") {
		t.Fatal("editor title missing")
	}
	// Edit and submit.
	m.ta.SetValue("Custom subject (#7)\n\nCustom body")
	cmd := key("submit")
	if cmd == nil || m.overlay != overlayNone || m.busy == "" || m.mergeMethod != "" {
		t.Fatalf("submit should fire the merge with a loader: busy=%q", m.busy)
	}
	m.busy = ""
	// Rebase keeps a yes/no confirmation.
	key("M")
	key("r")
	if m.mergeMethod != gh.Rebase || !strings.Contains(ansi.Strip(m.View().Content), "Rebase and merge PR #7") {
		t.Fatal("rebase confirmation missing")
	}
	key("n")
	if m.overlay != overlayNone || m.mergeMethod != "" {
		t.Fatal("n should cancel")
	}
	key("M")
	key("r")
	if key("y") == nil || m.overlay != overlayNone {
		t.Fatal("y should rebase-merge")
	}
	if !strings.Contains(ansi.Strip(m.renderHeader(200)[1]), "BLOCKED") {
		t.Fatal("header should show the merge state")
	}
	// Merge-commit default message.
	pr := &gh.PR{Number: 9, Title: "Do it", HeadRefName: "feat", HeadRepoOwner: "alice"}
	if s, b := gh.DefaultMergeMessage(pr, gh.MergeCommit); s != "Merge pull request #9 from alice/feat" || b != "Do it" {
		t.Fatalf("merge default %q %q", s, b)
	}
	if s, b := splitMessage("  Subject\r\n\nline1\nline2\n"); s != "Subject" || b != "line1\nline2" {
		t.Fatalf("splitMessage %q %q", s, b)
	}
}

func TestPickerStateFilter(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 0, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(prListMsg{prs: []gh.PRSummary{{Number: 1, Title: "one", State: "OPEN"}}})
	if m.listState != "open" || !strings.HasPrefix(m.list.Title, "Open pull requests") {
		t.Fatalf("default state: %q %q", m.listState, m.list.Title)
	}
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.listState != "closed" || cmd == nil || !strings.Contains(m.busy, "closed") || !strings.HasPrefix(m.list.Title, "Closed") {
		t.Fatalf("s should switch to closed and reload: %q busy=%q", m.listState, m.busy)
	}
	m.Update(prListMsg{prs: []gh.PRSummary{{Number: 2, Title: "two", State: "MERGED"}}})
	if it, ok := m.list.SelectedItem().(prItem); !ok || !strings.HasPrefix(it.Description(), "MERGED · ") {
		t.Fatal("non-open PRs should show their state")
	}
	for _, want := range []string{"merged", "all", "open"} {
		m.handleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
		if m.listState != want {
			t.Fatalf("cycle: got %q want %q", m.listState, want)
		}
	}
	// A failed reload of a non-default filter is a status, not fatal.
	m.listState = "closed"
	m.Update(prListMsg{err: errString("nope")})
	if m.fatal != nil || !m.statusErr {
		t.Fatal("list error should be shown in the status bar")
	}
	m.SetListState("merged")
	if m.listState != "merged" || !strings.HasPrefix(m.list.Title, "Merged") {
		t.Fatal("SetListState")
	}
	m.SetListState("bogus")
	if m.listState != "merged" {
		t.Fatal("invalid state ignored")
	}
}

func TestBackToList(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 0, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(prListMsg{prs: []gh.PRSummary{{Number: 7, Title: "one"}}})
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenDiff || !m.fromPicker {
		t.Fatal("enter should open the PR from the picker")
	}
	m.pr = &gh.PR{Number: 7}
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(sample)})
	// q returns to the list when opened from it.
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.screen != screenPicker || cmd == nil || m.pr != nil || len(m.files) != 0 {
		t.Fatalf("q should go back to the list and reset state: screen=%v", m.screen)
	}
	if !strings.Contains(m.busy, "Loading pull requests") {
		t.Fatalf("list should refresh with a loader, busy=%q", m.busy)
	}
	// Opened directly: q quits, b goes back.
	d := newTestModel(t)
	_, cmd = d.handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if d.screen != screenDiff || cmd == nil {
		t.Fatal("q should quit when not opened from the picker")
	}
	d.handleKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if d.screen != screenPicker {
		t.Fatal("b should always return to the list")
	}
}

func TestCloseReopen(t *testing.T) {
	m := newTestModel(t)
	key := func(k string) tea.Cmd {
		_, cmd := m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return cmd
	}
	key("X")
	if m.overlay != overlayState || !strings.Contains(ansi.Strip(m.View().Content), "Close PR #7 without merging?") {
		t.Fatal("X on an open PR should ask to close")
	}
	key("n")
	if m.overlay != overlayNone {
		t.Fatal("n cancels")
	}
	key("X")
	key("d")
	if cmd := key("y"); cmd == nil || m.overlay != overlayNone || !strings.Contains(m.busy, "Close PR") {
		t.Fatalf("y should close with a loader: busy=%q", m.busy)
	}
	m.busy = ""
	m.pr.State = "CLOSED"
	key("X")
	if !strings.Contains(ansi.Strip(m.View().Content), "Reopen PR #7?") {
		t.Fatal("X on a closed PR should ask to reopen")
	}
	if cmd := key("y"); cmd == nil || !strings.Contains(m.busy, "Reopen PR") {
		t.Fatalf("y should reopen: busy=%q", m.busy)
	}
	m.busy = ""
	if key("M"); m.overlay == overlayMerge || !m.statusErr {
		t.Fatal("merging a closed PR should be refused")
	}
	m.pr.State = "MERGED"
	if key("X"); m.overlay == overlayState || !m.statusErr {
		t.Fatal("reopening a merged PR should be refused")
	}
}

func TestTopBottomKeys(t *testing.T) {
	m := newTreeModel(t)
	press := func(k string) {
		switch k {
		case "tab":
			m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
		case "ctrl+f", "ctrl+b":
			m.handleKey(tea.KeyPressMsg{Code: rune(k[5]), Mod: tea.ModCtrl})
		default:
			m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		}
	}
	// Tree mode: G selects the last node (README.md), g the first (a directory).
	press("tab")
	press("G")
	if m.treeSel != "README.md" || m.files[m.fileIdx].Path() != "README.md" {
		t.Fatalf("G in tree: sel=%q file=%s", m.treeSel, m.files[m.fileIdx].Path())
	}
	press("g")
	if m.treeSel != "cmd/x/deep" {
		t.Fatalf("g in tree should select the first node, got %q", m.treeSel)
	}
	// Flat mode: g/G select first/last file.
	press("t")
	press("G")
	if m.fileIdx != len(m.files)-1 {
		t.Fatalf("G flat -> last file, got %d", m.fileIdx)
	}
	press("g")
	if m.fileIdx != 0 {
		t.Fatalf("g flat -> first file, got %d", m.fileIdx)
	}
	// Diff pane: g/G and full-page keys.
	press("tab")
	d := newTestModel(t)
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	d.handleKey(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if d.cursor != len(d.rows)-1 {
		t.Fatal("G -> last row")
	}
	d.handleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if d.cursor != 0 {
		t.Fatal("g -> first row")
	}
	d.handleKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if d.cursor != d.viewportH() {
		t.Fatalf("ctrl+f should move a full page (%d), got %d", d.viewportH(), d.cursor)
	}
	d.handleKey(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	if d.cursor != 0 {
		t.Fatal("ctrl+b should move back a full page")
	}
}

func TestLOpensFileFromPanel(t *testing.T) {
	m := newTreeModel(t)
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.treeSel = "internal/ui/render.go"
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.filesFocused || m.files[m.fileIdx].Path() != "internal/ui/render.go" {
		t.Fatalf("l on a file should open it and focus the diff: focused=%v file=%s", m.filesFocused, m.files[m.fileIdx].Path())
	}
	// l on a collapsed directory still expands it and keeps focus.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m.collapsed["internal/ui"] = true
	m.rebuildTree()
	m.treeSel = "internal/ui"
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.collapsed["internal/ui"] || !m.filesFocused {
		t.Fatal("l on a directory should expand it")
	}
	// Flat mode.
	m.handleKey(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.filesFocused {
		t.Fatal("l in flat mode should return focus to the diff")
	}
}

func TestHLBetweenTreeAndDiff(t *testing.T) {
	m := newTreeModel(t)
	// h in the diff focuses the file tree (and shows it if hidden).
	m.showFiles = false
	m.handleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !m.filesFocused || !m.showFiles || m.fileIdx != 0 {
		t.Fatalf("h should focus the tree without changing file: focused=%v show=%v idx=%d", m.filesFocused, m.showFiles, m.fileIdx)
	}
	// l on a file in the tree opens it and returns to the diff.
	m.treeSel = "internal/ui/render.go"
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.filesFocused || m.files[m.fileIdx].Path() != "internal/ui/render.go" {
		t.Fatalf("l on a file should open it: focused=%v file=%s", m.filesFocused, m.files[m.fileIdx].Path())
	}
	// l on a collapsed directory expands it and keeps focus in the tree.
	m.handleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m.collapsed["internal/ui"] = true
	m.rebuildTree()
	m.treeSel = "internal/ui"
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.collapsed["internal/ui"] || !m.filesFocused {
		t.Fatal("l on a directory should expand it and stay in the tree")
	}
	// [ still moves to the previous file from the diff.
	m.filesFocused = false
	m.selectFile(2)
	m.handleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if m.fileIdx != 1 {
		t.Fatalf("[ should go to the previous file, got %d", m.fileIdx)
	}
}

func TestAutoFoldViewedDirs(t *testing.T) {
	m := newTreeModel(t)
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.SetStore(st)
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(treeSample)})
	byPath := func(p string) int {
		for i := range m.files {
			if m.files[i].Path() == p {
				return i
			}
		}
		t.Fatalf("no file %s", p)
		return -1
	}
	mark := func() { m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"}) }

	// Single-file compacted dir folds immediately and becomes the selection.
	m.selectFile(byPath("cmd/x/deep/main.go"))
	mark()
	if !m.collapsed["cmd/x/deep"] || m.treeSel != "cmd/x/deep" {
		t.Fatalf("cmd/x/deep should fold: collapsed=%v sel=%q", m.collapsed, m.treeSel)
	}
	// Partially viewed dir stays open.
	m.selectFile(byPath("internal/ui/model.go"))
	mark()
	if m.collapsed["internal/ui"] {
		t.Fatal("internal/ui still has an unviewed file and must stay open")
	}
	m.selectFile(byPath("internal/ui/render.go"))
	mark()
	if !m.collapsed["internal/ui"] || m.treeSel != "internal/ui" {
		t.Fatalf("internal/ui should fold once both files are viewed: %v %q", m.collapsed, m.treeSel)
	}
	if got := strings.Join(labels(m.treeNodes), "|"); got != "cmd/x/deep/|internal/ui/|README.md" {
		t.Fatalf("tree after folding: %s", got)
	}
	// Unmarking the current file re-expands its folder.
	mark()
	if m.collapsed["internal/ui"] || m.isViewed("internal/ui/render.go") {
		t.Fatal("unmark should expand the folder")
	}
	// Flat mode never folds.
	m.tree = false
	mark()
	if m.collapsed["internal/ui"] {
		t.Fatal("flat mode must not touch collapse state")
	}
}

const wideSample = `diff --git a/w.txt b/w.txt
--- a/w.txt
+++ b/w.txt
@@ -1,2 +1,2 @@
 short
-START_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_END
+START_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_0123456789_NEW
`

func TestHorizontalScroll(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.pr = &gh.PR{Number: 7, HeadRefOid: "abc"}
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(wideSample)})
	press := func(k string) { m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k}) }
	view := func() string { return ansi.Strip(m.View().Content) }

	if m.maxHScroll() == 0 {
		t.Fatalf("line should overflow: maxLineW=%d content=%d", m.maxLineW, m.contentWidth())
	}
	if strings.Contains(view(), "_END") || !strings.Contains(view(), "START_") {
		t.Fatal("initially the start of the line is visible and the end is cut")
	}
	// l pans right until the end is visible, then stops.
	for i := 0; i < 20 && m.hscroll < m.maxHScroll(); i++ {
		press("l")
	}
	if m.hscroll != m.maxHScroll() || !strings.Contains(view(), "_END") || strings.Contains(view(), "START_") {
		t.Fatalf("after panning right the end should be visible: hscroll=%d\n%s", m.hscroll, view())
	}
	if !strings.Contains(view(), "→ col") {
		t.Fatal("header should indicate the scroll offset")
	}
	press("l")
	if m.hscroll != m.maxHScroll() || m.filesFocused || m.fileIdx != 0 {
		t.Fatal("l at the right edge does nothing")
	}
	// h pans back; only at the left edge does it go to the file tree.
	for i := 0; i < 20 && m.hscroll > 0; i++ {
		press("h")
		if m.filesFocused {
			t.Fatal("h must not switch focus while scrolled")
		}
	}
	if m.hscroll != 0 || !strings.Contains(view(), "START_") {
		t.Fatal("h should return to the left edge")
	}
	press("h")
	if !m.filesFocused {
		t.Fatal("h at the left edge focuses the file tree")
	}
	// Split view pans both halves; switching layout resets the offset.
	m.filesFocused = false
	press("l")
	press("s")
	if m.hscroll != 0 || !m.split {
		t.Fatal("toggling split resets scroll")
	}
	press("l")
	if m.hscroll == 0 || strings.Contains(view(), "START_") {
		t.Fatalf("split view should pan too: hscroll=%d", m.hscroll)
	}
	for _, l := range lines(m.View().Content) {
		if ansi.StringWidth(l) != 80 {
			t.Fatalf("width drift while scrolled: %q", ansi.Strip(l))
		}
	}
	// A file without overflow (in unified view): l does nothing, h goes straight to the tree.
	m.split = false
	m.hscroll = 0
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(sample)})
	if m.maxHScroll() != 0 {
		t.Fatalf("sample should fit: maxLineW=%d content=%d", m.maxLineW, m.contentWidth())
	}
	press("l")
	if m.hscroll != 0 {
		t.Fatal("no overflow, no scroll")
	}
	press("h")
	if !m.filesFocused {
		t.Fatal("h without overflow focuses the tree")
	}
}

func TestResumeLastPosition(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// First session: go to b.py, move the cursor, quit.
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.SetStore(st)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.pr = &gh.PR{Number: 7, HeadRefOid: "abc"}
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(sample)})
	if m.fileIdx != 0 {
		t.Fatal("nothing to resume on first open")
	}
	m.selectFile(1)
	m.cursor = 3 // "+    return 2" -> new line 2 (row 2 is the deletion)
	m.handleKey(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	pos, ok := st.GetLast(state.PRKey("o/r", 7))
	if !ok || pos.Path != "b.py" || pos.Line != 2 || pos.Side != "RIGHT" {
		t.Fatalf("position not saved: %+v ok=%v", pos, ok)
	}

	// Second session with a fresh store instance resumes there.
	st2, _ := state.Open(dir)
	m2 := New(&gh.Client{Repo: "o/r"}, 7, "")
	m2.SetStore(st2)
	m2.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m2.pr = &gh.PR{Number: 7, HeadRefOid: "abc"}
	m2.pending = 1
	m2.Update(diffMsg{files: diff.Parse(sample)})
	if m2.files[m2.fileIdx].Path() != "b.py" {
		t.Fatalf("should resume on b.py, got %s", m2.files[m2.fileIdx].Path())
	}
	if _, n, _ := m2.rows[m2.cursor].nums(); n != 2 {
		t.Fatalf("should resume on new line 2, got row %d", m2.cursor)
	}
	if !strings.Contains(m2.status, "Resumed at b.py") {
		t.Fatalf("status %q", m2.status)
	}
	// A refresh keeps the current place instead of jumping again.
	m2.selectFile(0)
	m2.pending = 1
	m2.Update(diffMsg{files: diff.Parse(sample)})
	if m2.fileIdx != 0 {
		t.Fatal("refresh must not re-apply the remembered position")
	}
	// A remembered file that left the PR is ignored.
	st2.SetLast(state.PRKey("o/r", 7), state.Position{Path: "gone.go", Line: 1})
	m3 := New(&gh.Client{Repo: "o/r"}, 7, "")
	m3.SetStore(st2)
	m3.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m3.pr = &gh.PR{Number: 7}
	m3.pending = 1
	m3.Update(diffMsg{files: diff.Parse(sample)})
	if m3.fileIdx != 0 || m3.status != "" {
		t.Fatalf("missing file should be ignored quietly: idx=%d status=%q", m3.fileIdx, m3.status)
	}
	// Going back to the list also saves.
	m3.selectFile(1)
	m3.backToList()
	if p, _ := st2.GetLast(state.PRKey("o/r", 7)); p.Path != "b.py" {
		t.Fatalf("backToList should save, got %+v", p)
	}
}

func TestFoldViewedDirsOnOpen(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := diff.Parse(treeSample)
	pr := state.PRKey("o/r", 7)
	fp := func(p string) string {
		for i := range files {
			if files[i].Path() == p {
				return files[i].Fingerprint()
			}
		}
		return ""
	}
	// cmd/x/deep fully viewed; internal/ui only half viewed.
	st.Set(pr, "cmd/x/deep/main.go", state.Viewed{ViewedAt: time.Now(), Fingerprint: fp("cmd/x/deep/main.go")})
	st.Set(pr, "internal/ui/model.go", state.Viewed{ViewedAt: time.Now(), Fingerprint: fp("internal/ui/model.go")})

	open := func() *Model {
		m := New(&gh.Client{Repo: "o/r"}, 7, "")
		m.SetStore(st)
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		m.pr = &gh.PR{Number: 7}
		m.pending = 1
		m.Update(diffMsg{files: diff.Parse(treeSample)})
		return m
	}
	m := open()
	if !m.collapsed["cmd/x/deep"] || m.collapsed["internal/ui"] {
		t.Fatalf("only fully viewed folders fold on open: %v", m.collapsed)
	}
	if got := strings.Join(labels(m.treeNodes), "|"); got != "cmd/x/deep/|internal/ui/| model.go| render.go|README.md" {
		t.Fatalf("tree on open: %s", got)
	}
	// If the current file sits in a folded folder, the folder is selected.
	st.SetLast(pr, state.Position{Path: "cmd/x/deep/main.go"})
	m = open()
	if m.files[m.fileIdx].Path() != "cmd/x/deep/main.go" || m.collapsed["cmd/x/deep"] {
		t.Fatalf("resuming inside a folded folder reveals the file: idx=%d collapsed=%v", m.fileIdx, m.collapsed)
	}
	// Without a remembered position, file 0 (README.md) is visible and nothing needs fixing.
	delete(st.Last, pr)
	st.Set(pr, "README.md", state.Viewed{ViewedAt: time.Now(), Fingerprint: fp("README.md")})
	m = open()
	if m.treeSel != "README.md" || !m.collapsed["cmd/x/deep"] {
		t.Fatalf("sel=%q collapsed=%v", m.treeSel, m.collapsed)
	}
	// Refresh does not re-fold folders the user expanded.
	delete(m.collapsed, "cmd/x/deep")
	m.pending = 1
	m.Update(diffMsg{files: diff.Parse(treeSample)})
	if m.collapsed["cmd/x/deep"] {
		t.Fatal("refresh must keep the user's expanded folders")
	}
}

func TestDeleteComment(t *testing.T) {
	m := newTestModel(t)
	m.Update(userMsg{login: "bob"})
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		if k == "esc" {
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		} else {
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	// Not on a thread.
	m.cursor = 1
	press("d")
	if m.overlay != overlayNone || !m.statusErr {
		t.Fatal("d off a thread should be refused")
	}
	// Thread T2 (row 4) has only carol's comment: nothing of bob's.
	m.cursor = 4
	press("d")
	if m.overlay != overlayNone || !strings.Contains(m.status, "No comment of yours") {
		t.Fatalf("expected refusal, status=%q", m.status)
	}
	// Thread T1 (row 7): alice then bob -> bob's reply is offered.
	m.cursor = 7
	press("d")
	if m.overlay != overlayDelete || len(m.pickChoices) != 1 || m.pickChoices[0].Author != "bob" {
		t.Fatalf("delete overlay should target bob's comment: %v", m.pickChoices)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Delete @bob's comment “Style.”?") || !strings.Contains(plain, "✗ delete?") {
		t.Fatalf("confirmation and marker missing:\n%s", plain)
	}
	press("n")
	if m.overlay != overlayNone || m.pickTarget() != nil {
		t.Fatal("n cancels")
	}
	press("d")
	if cmd := press("y"); cmd == nil || m.overlay != overlayNone || !strings.Contains(m.busy, "Delete comment") {
		t.Fatalf("y should delete with a loader: busy=%q", m.busy)
	}
	m.busy = ""
	// Unknown login: every comment is offered, newest preselected, j/k choose.
	m.login = ""
	press("d")
	if len(m.pickChoices) != 2 || m.pickIdx != 1 || m.pickTarget().Author != "bob" {
		t.Fatalf("all comments offered, newest first: %d %d", len(m.pickChoices), m.pickIdx)
	}
	press("k")
	if m.pickTarget().Author != "alice" || !strings.Contains(ansi.Strip(m.View().Content), "choose (1/2)") {
		t.Fatal("k should move to alice's comment")
	}
	press("esc")
	if m.overlay != overlayNone {
		t.Fatal("esc cancels")
	}
}

func TestEditComment(t *testing.T) {
	m := newTestModel(t)
	m.Update(userMsg{login: "bob"})
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "ctrl+s":
			msg = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	// Not on a thread.
	m.cursor = 1
	press("e")
	if m.overlay != overlayNone || !strings.Contains(m.status, "to edit a comment") {
		t.Fatalf("e off a thread should be refused, status=%q", m.status)
	}
	// Thread T2 (row 4): nothing of bob's.
	m.cursor = 4
	press("e")
	if m.overlay != overlayNone || !strings.Contains(m.status, "No comment of yours") {
		t.Fatalf("expected refusal, status=%q", m.status)
	}
	// Thread T1 (row 7): bob's reply is offered with an edit marker.
	m.cursor = 7
	press("e")
	if m.overlay != overlayEdit || len(m.pickChoices) != 1 || m.pickTarget().Author != "bob" {
		t.Fatalf("edit picker should target bob's comment: %v", m.pickChoices)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Edit @bob's comment “Style.”?") || !strings.Contains(plain, "✎ edit?") || strings.Contains(plain, "delete?") {
		t.Fatalf("edit prompt and marker missing:\n%s", plain)
	}
	press("n")
	if m.overlay != overlayNone || m.pickTarget() != nil {
		t.Fatal("n cancels")
	}
	// y opens the editor pre-filled with the current body.
	press("e")
	press("y")
	if m.overlay != overlayInput || m.inKind != inputEdit || m.inEditID != 2 || m.ta.Value() != "Style." {
		t.Fatalf("y should open the editor with the body: overlay=%v kind=%v id=%d value=%q", m.overlay, m.inKind, m.inEditID, m.ta.Value())
	}
	if !strings.Contains(m.inputTitle, "Edit @bob's comment on main.go:4") {
		t.Fatalf("title=%q", m.inputTitle)
	}
	// Emptying the body is refused; otherwise submit edits with a loader.
	m.ta.SetValue("")
	press("ctrl+s")
	if m.overlay != overlayInput || !strings.Contains(m.status, "empty") {
		t.Fatal("empty edit should be refused")
	}
	m.ta.SetValue("Style nit.")
	if cmd := press("ctrl+s"); cmd == nil || m.overlay != overlayNone || !strings.Contains(m.busy, "Edit comment") {
		t.Fatalf("submit should edit with a loader: busy=%q", m.busy)
	}
	m.busy = ""
	// The picker keys also work under e, and e closes it again.
	m.login = ""
	press("e")
	press("k")
	if m.pickTarget().Author != "alice" || !strings.Contains(ansi.Strip(m.View().Content), "choose (1/2)") {
		t.Fatal("k should move to alice's comment")
	}
	press("e")
	if m.overlay != overlayNone {
		t.Fatal("e toggles the picker closed")
	}
}

func TestOpenInEditor(t *testing.T) {
	m := newTestModel(t)
	var opened []string
	server := false
	editorHasServer = func(id string) bool { return server && id == "PR_test7" }
	editorOpen = func(id string, path string) error {
		opened = append(opened, fmt.Sprintf("%s:%s", id, path))
		return nil
	}
	t.Cleanup(func() { editorHasServer, editorOpen = editor.HasServer, editor.Open })
	press := func(k string) tea.Cmd {
		_, cmd := m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return cmd
	}
	// No socket for this PR: o does nothing at all.
	if cmd := press("o"); cmd != nil || m.busy != "" || m.status != "" {
		t.Fatalf("o without a server should be silent: busy=%q status=%q", m.busy, m.status)
	}
	// With a server the current file is handed over.
	server = true
	cmd := press("o")
	if cmd == nil || !strings.Contains(m.busy, "Open in nvim") {
		t.Fatalf("o should open in nvim: busy=%q", m.busy)
	}
	runBatch(cmd)
	if !reflect.DeepEqual(opened, []string{"PR_test7:main.go"}) {
		t.Fatalf("opened = %v", opened)
	}
	m.busy = ""
	m.selectFile(1)
	runBatch(press("o"))
	if len(opened) != 2 || opened[1] != "PR_test7:b.py" {
		t.Fatalf("opened = %v", opened)
	}
	// O opens the browser.
	m.busy = ""
	press("O")
	if !strings.Contains(m.busy, "Open in browser") {
		t.Fatalf("O should open the browser: busy=%q", m.busy)
	}
}

// runBatch executes the leaf commands of a tea.Batch synchronously.
func runBatch(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runBatch(c)
		}
	}
}

func TestDebugKeys(t *testing.T) {
	m := newTestModel(t)
	m.SetDebugKeys(true)
	m.cursor = 1
	m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) // empty submit -> error status
	plain := ansi.Strip(m.View().Content)
	if m.lastKey != "ctrl+s" || !strings.Contains(plain, "key: ctrl+s") {
		t.Fatalf("key name should show: %q", m.lastKey)
	}
	if !strings.Contains(plain, "Comment body is empty") {
		t.Fatal("debug key display must not hide real status messages")
	}
}
