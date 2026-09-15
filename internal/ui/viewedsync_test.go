package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
	"ghpr/internal/state"
)

func TestRemoteViewedMerge(t *testing.T) {
	m := newTestModel(t)
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.SetStore(st)
	// Paths not in the diff are ignored; b.py gets a local mark with the
	// current fingerprint.
	m.Update(viewedMsg{paths: []string{"b.py", "not/in/diff.go"}})
	if !m.isViewed("b.py") || m.isViewed("main.go") {
		t.Fatalf("b.py should be marked, main.go not: %v %v", m.isViewed("b.py"), m.isViewed("main.go"))
	}
	if v, _ := m.viewedInfo("b.py"); v.Fingerprint != m.fingerprints[1] || v.HeadSHA != "abc" {
		t.Fatalf("mark should carry fingerprint and head sha: %+v", v)
	}
	if !strings.Contains(m.status, "1 file(s) marked viewed on GitHub") {
		t.Fatalf("status=%q", m.status)
	}
	// Already-marked files are left alone and produce no message.
	m.status = ""
	m.Update(viewedMsg{paths: []string{"b.py"}})
	if m.status != "" {
		t.Fatalf("no new marks should be silent, status=%q", m.status)
	}
	// An error is reported, not fatal.
	m.Update(viewedMsg{err: errString("boom")})
	if !m.statusErr || !strings.Contains(m.status, "boom") || m.screen != screenDiff {
		t.Fatalf("error should show as status: %q", m.status)
	}
	// Without a store nothing is fetched or merged.
	m.SetStore(nil)
	if m.fetchViewed() != nil {
		t.Fatal("no store: no fetch")
	}
	m.Update(viewedMsg{paths: []string{"main.go"}})
	if m.isViewed("main.go") {
		t.Fatal("no store: nothing merged")
	}
}

func TestRemoteViewedBeforeDiff(t *testing.T) {
	// GitHub's answer can arrive before the diff: it is held and merged when
	// the files land.
	m := New(&gh.Client{Repo: "o/r"}, 7, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	st, _ := state.Open(t.TempDir())
	m.SetStore(st)
	m.pr = &gh.PR{ID: "PR_x", Number: 7, HeadRefOid: "abc"}
	m.pending = 2
	m.Update(viewedMsg{paths: []string{"main.go"}})
	if m.remoteViewed == nil || m.isViewed("main.go") {
		t.Fatal("paths should wait for the diff")
	}
	m.Update(diffMsg{files: diff.Parse(sample)})
	if !m.isViewed("main.go") || m.remoteViewed != nil {
		t.Fatalf("merge after diff: viewed=%v pending=%v", m.isViewed("main.go"), m.remoteViewed)
	}
	// m pushes the toggle to GitHub only when the PR's node id is known.
	if m.pushViewed("main.go", true) == nil {
		t.Fatal("push should be scheduled with a PR id")
	}
	m.pr.ID = ""
	if m.pushViewed("main.go", true) != nil {
		t.Fatal("no PR id: no push")
	}
	// A failed push is reported.
	m.Update(viewedSyncMsg{path: "main.go", err: errString("nope")})
	if !m.statusErr || !strings.Contains(m.status, "main.go") || !strings.Contains(m.status, "nope") {
		t.Fatalf("status=%q", m.status)
	}
}
