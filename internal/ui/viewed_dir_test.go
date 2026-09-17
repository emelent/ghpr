package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
	"ghpr/internal/state"
)

func TestViewedFolder(t *testing.T) {
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.pr = &gh.PR{ID: "PR_x", Number: 7, HeadRefOid: "abc"}
	st, _ := state.Open(t.TempDir())
	m.SetStore(st)
	m.pending = 2
	m.Update(diffMsg{files: diff.Parse(treeSample)})
	m.rebuildTree()
	press := func(k string) tea.Cmd {
		_, cmd := m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return cmd
	}
	// Select the internal/ui folder in the tree and press m: both files
	// beneath it are marked, the folder folds and stays selected.
	m.filesFocused = true
	m.treeSel = "internal/ui"
	if cmd := press("m"); cmd == nil {
		t.Fatal("m on a folder should mark and push")
	}
	if !m.isViewed("internal/ui/model.go") || !m.isViewed("internal/ui/render.go") || m.isViewed("README.md") || m.isViewed("cmd/x/deep/main.go") {
		t.Fatal("only the folder's files should be viewed")
	}
	if !m.collapsed["internal/ui"] || m.treeSel != "internal/ui" {
		t.Fatalf("folder should fold and stay selected: collapsed=%v sel=%q", m.collapsed["internal/ui"], m.treeSel)
	}
	if !strings.Contains(m.status, "Viewed 2 file(s) in internal/ui/ (2/4)") {
		t.Fatalf("status=%q", m.status)
	}
	// The marks are on disk.
	st2, _ := state.Open(st.Path()[:len(st.Path())-len("/viewed.json")])
	if _, ok := st2.Get(m.prKey(), "internal/ui/render.go"); !ok {
		t.Fatal("folder marks should be saved")
	}
	// A second m unmarks them all and unfolds the folder.
	press("m")
	if m.isViewed("internal/ui/model.go") || m.isViewed("internal/ui/render.go") || m.collapsed["internal/ui"] {
		t.Fatal("second m should unmark and unfold")
	}
	if !strings.Contains(m.status, "Unmarked 2 file(s) in internal/ui/") {
		t.Fatalf("status=%q", m.status)
	}
	// Partially viewed folder: m marks the rest.
	m.filesFocused = false
	m.selectFile(1) // internal/ui/model.go
	press("m")
	m.filesFocused = true
	m.treeSel = "internal/ui"
	press("m")
	if !m.isViewed("internal/ui/model.go") || !m.isViewed("internal/ui/render.go") {
		t.Fatal("m on a partly viewed folder marks the remaining files")
	}
	if !strings.Contains(m.status, "Viewed 1 file(s) in internal/ui/") {
		t.Fatalf("status=%q", m.status)
	}
	// A compacted chain node (cmd/x/deep) covers its files too.
	m.treeSel = "cmd/x/deep"
	press("m")
	if !m.isViewed("cmd/x/deep/main.go") {
		t.Fatal("compacted folder node should mark its file")
	}
	// With the tree unfocused m still toggles the current file only.
	m.filesFocused = false
	m.selectFile(0) // README.md
	press("m")
	if !m.isViewed("README.md") || m.viewedCount() != 4 {
		t.Fatalf("file toggle: viewed=%d", m.viewedCount())
	}
}
