package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/gh"
)

func TestCommentsScreen(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	// i opens the screen with threads in file/line order: T2 (main.go:3),
	// T1 (main.go:4), T3 (b.py, outdated).
	m.cursor = 0
	press("i")
	if m.screen != screenComments || len(m.cmThreads) != 3 || m.cmIdx != 0 {
		t.Fatalf("screen=%v threads=%d idx=%d", m.screen, len(m.cmThreads), m.cmIdx)
	}
	if ids := []string{m.cmThreads[0].ID, m.cmThreads[1].ID, m.cmThreads[2].ID}; strings.Join(ids, ",") != "T2,T1,T3" {
		t.Fatalf("order = %v", ids)
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Comments (3 threads · 2 open)", "main.go:3", "@carol", "resolved", "- import \"fmt\"", "old import", "main.go:4", "@alice", "+ \t\"fmt\"", "Why the parens?", "b.py:40 (orig)", "outdated", "(line no longer in the diff)", "gone"} {
		if !strings.Contains(plain, strings.ReplaceAll(want, "\t", "    ")) {
			t.Fatalf("missing %q in:\n%s", want, plain)
		}
	}
	// A multi-line thread shows every line of its range, in order.
	rng := gh.Thread{ID: "T4", Path: "main.go", StartLine: 3, Line: 5, DiffSide: "RIGHT", Comments: []gh.Comment{{DatabaseID: 9, Author: "erin", Body: "whole import block", CreatedAt: time.Now()}}}
	m.Update(threadsMsg{threads: append(append([]gh.Thread{}, m.threads...), rng)})
	if len(m.cmThreads) != 4 || m.cmThreads[2].ID != "T4" {
		t.Fatalf("range thread should sort by its last line: %v", m.cmThreads)
	}
	lines, hidden := m.threadSnippet(m.cmThreads[2])
	if hidden != 0 || len(lines) != 3 || lines[0].text != "import (" || lines[1].text != `    "fmt"` || lines[2].text != ")" {
		t.Fatalf("snippet = %+v hidden=%d", lines, hidden)
	}
	if m.commentItemHeight(m.cmThreads[2]) != 6 || m.commentItemHeight(m.cmThreads[0]) != 4 {
		t.Fatalf("heights: %d %d", m.commentItemHeight(m.cmThreads[2]), m.commentItemHeight(m.cmThreads[0]))
	}
	plain = ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "main.go:3-5") || !strings.Contains(plain, "+ import (") || !strings.Contains(plain, "+ )") {
		t.Fatalf("range rendering missing:\n%s", plain)
	}
	// Long ranges are capped with a count of what is left out.
	big := gh.Thread{ID: "T5", Path: "main.go", StartLine: 1, Line: 8, DiffSide: "RIGHT", Comments: []gh.Comment{{DatabaseID: 10, Author: "erin", Body: "x", CreatedAt: time.Now()}}}
	m.Update(threadsMsg{threads: append(append([]gh.Thread{}, m.threads...), big)})
	lines, hidden = m.threadSnippet(&m.threads[len(m.threads)-1])
	if len(lines) != 8 || hidden != 0 {
		t.Fatalf("8-line range fits the cap exactly: %d hidden=%d", len(lines), hidden)
	}
	m.Update(threadsMsg{threads: append(append([]gh.Thread{}, m.threads[:3]...), rng)})
	// j/k/G/g move; l opens the thread in the diff and h comes back.
	press("j")
	press("j")
	press("k")
	if m.cmIdx != 1 {
		t.Fatalf("idx=%d", m.cmIdx)
	}
	press("l")
	if m.screen != screenDiff || m.backTo != screenComments || m.fileIdx != 0 {
		t.Fatalf("l should open the diff: screen=%v back=%v file=%d", m.screen, m.backTo, m.fileIdx)
	}
	if r := m.currentRow(); r == nil || r.kind != rowThread || r.thread.ID != "T1" {
		t.Fatalf("cursor should be on T1: row=%d", m.cursor)
	}
	press("h")
	if m.screen != screenComments || m.cmIdx != 1 {
		t.Fatalf("h should return to the comments: screen=%v idx=%d", m.screen, m.cmIdx)
	}
	// G then enter jumps into the other file; esc from the diff does not
	// leave the diff, esc from the list does.
	press("G")
	press("enter")
	if m.screen != screenDiff || m.fileIdx != 1 || m.currentRow().kind != rowThread || m.currentRow().thread.ID != "T3" {
		t.Fatalf("enter should open T3 in b.py: file=%d row=%d", m.fileIdx, m.cursor)
	}
	press("esc")
	if m.screen != screenDiff {
		t.Fatal("esc in the diff stays in the diff")
	}
	press("h")
	press("esc")
	if m.screen != screenDiff || m.hasBack() {
		t.Fatalf("esc closes the comments screen: screen=%v back=%v", m.screen, m.backTo)
	}
	// Without the comments screen, h at the left edge focuses the file tree.
	press("h")
	if m.screen != screenDiff || !m.filesFocused {
		t.Fatal("h should focus the file tree when not coming from comments")
	}
	m.filesFocused = false
	// Opening from a thread row preselects it.
	m.selectFile(0)
	m.cursor = 7 // T1
	press("i")
	if m.cmIdx != 1 {
		t.Fatalf("thread under cursor should be preselected: idx=%d", m.cmIdx)
	}
	// Mouse: click selects, second click opens; wheel moves. Items vary in
	// height (T2 4, T1 4, T4 6, T3 4 lines), so aim inside the third item.
	m.cmScroll = 0
	y := headerH + paneHeaderH + 4 + 4 + 5 // last code line of T4
	m.Update(tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
	if m.screen != screenComments || m.cmIdx != 2 || m.cmThreads[2].ID != "T4" {
		t.Fatalf("click should select the third thread: idx=%d", m.cmIdx)
	}
	m.Update(tea.MouseWheelMsg{X: 3, Y: y, Button: tea.MouseWheelUp})
	if m.cmIdx != 1 {
		t.Fatalf("wheel up: idx=%d", m.cmIdx)
	}
	m.Update(tea.MouseClickMsg{X: 3, Y: headerH + paneHeaderH + 4 + 2, Button: tea.MouseLeft})
	if m.screen != screenDiff || m.currentRow().thread.ID != "T1" {
		t.Fatal("clicking the selected thread opens it")
	}
	// No threads: i reports it and stays in the diff.
	m.backTo = screenDiff
	m.Update(threadsMsg{threads: nil})
	press("i")
	if m.screen != screenDiff || !strings.Contains(m.status, "No review threads") {
		t.Fatalf("expected refusal, status=%q", m.status)
	}
}
