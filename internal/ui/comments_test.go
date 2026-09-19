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
	// i opens the screen without the resolved T2; t shows it too, and the
	// threads come in file/line order: T2 (main.go:3), T1 (main.go:4),
	// T3 (b.py, outdated).
	m.cursor = 0
	press("i")
	if m.screen != screenComments || len(m.cmThreads) != 2 || m.cmIdx != 0 || m.cmThreads[0].ID != "T1" {
		t.Fatalf("screen=%v threads=%d idx=%d", m.screen, len(m.cmThreads), m.cmIdx)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Comments (2 open · 1 resolved hidden)") || strings.Contains(plain, "@carol") {
		t.Fatalf("resolved thread should be hidden:\n%s", plain)
	}
	press("t")
	if len(m.cmThreads) != 3 || m.cmIdx != 1 || m.cmThreads[1].ID != "T1" {
		t.Fatalf("t should show resolved threads and keep the selection: threads=%d idx=%d", len(m.cmThreads), m.cmIdx)
	}
	m.cmIdx = 0
	if ids := []string{m.cmThreads[0].ID, m.cmThreads[1].ID, m.cmThreads[2].ID}; strings.Join(ids, ",") != "T2,T1,T3" {
		t.Fatalf("order = %v", ids)
	}
	plain = ansi.Strip(m.View().Content)
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

func TestCommentsResolve(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		_, cmd := m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return cmd
	}
	press("i")
	if m.screen != screenComments || m.cmThreads[m.cmIdx].ID != "T1" {
		t.Fatalf("screen=%v idx=%d", m.screen, m.cmIdx)
	}
	// x starts the resolve for the selected thread; a second x on the same
	// thread while the answer is still out is refused.
	if cmd := press("x"); cmd == nil || !strings.HasPrefix(m.busy, "Resolve thread") {
		t.Fatalf("x should resolve the selected thread: cmd=%v busy=%q", cmd != nil, m.busy)
	}
	press("x")
	if len(m.running) != 1 || !strings.Contains(m.status, "already running") {
		t.Fatalf("a repeat should be refused: running=%d status=%q", len(m.running), m.status)
	}
	// The refresh after resolving drops the thread from the list and the
	// selection moves to the next open one.
	threads := append([]gh.Thread{}, m.threads...)
	threads[0].IsResolved = true // T1
	m.Update(actionMsg{seq: m.running[0].seq, label: "Resolve thread"})
	m.Update(threadsMsg{seq: 99, threads: threads})
	if len(m.cmThreads) != 1 || m.cmThreads[0].ID != "T3" || m.cmIdx != 0 {
		t.Fatalf("resolved thread should leave the list: %d idx=%d", len(m.cmThreads), m.cmIdx)
	}
	// With everything resolved the screen says so instead of closing.
	threads[2].IsResolved = true // T3
	m.Update(threadsMsg{seq: 100, threads: threads})
	plain := ansi.Strip(m.View().Content)
	if len(m.cmThreads) != 0 || !strings.Contains(plain, "Comments (0 open · 3 resolved hidden)") || !strings.Contains(plain, "Every thread is resolved · t shows them") {
		t.Fatalf("empty state:\n%s", plain)
	}
	if press("x") != nil {
		t.Fatal("x with no thread selected should do nothing")
	}
	// t reveals them; x on a resolved thread unresolves it.
	press("t")
	if len(m.cmThreads) != 3 || !strings.Contains(ansi.Strip(m.View().Content), "hide resolved") {
		t.Fatalf("t should list every thread: %d", len(m.cmThreads))
	}
	if cmd := press("x"); cmd == nil || !strings.HasPrefix(m.busy, "Unresolve thread") {
		t.Fatalf("x on a resolved thread should unresolve: busy=%q", m.busy)
	}
	// Reopening the screen with no threads at all still refuses.
	m.closeComments()
	m.Update(threadsMsg{seq: 101, threads: nil})
	press("i")
	if m.screen != screenDiff || !strings.Contains(m.status, "No review threads") {
		t.Fatalf("expected refusal, status=%q", m.status)
	}
}

func TestCommentsSearch(t *testing.T) {
	m := newTestModel(t)
	key := func(k string) tea.KeyPressMsg {
		switch k {
		case "esc":
			return tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			return tea.KeyPressMsg{Code: tea.KeyEnter}
		case "backspace":
			return tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+u":
			return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
		case "down":
			return tea.KeyPressMsg{Code: tea.KeyDown}
		}
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
	press := func(k string) { m.handleKey(key(k)) }
	typ := func(s string) {
		for _, r := range s {
			m.handleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	ids := func() string {
		out := make([]string, len(m.cmThreads))
		for i, th := range m.cmThreads {
			out[i] = th.ID
		}
		return strings.Join(out, ",")
	}
	// The screen opens on the two open threads; / starts filtering.
	press("i")
	if ids() != "T1,T3" {
		t.Fatalf("threads = %s", ids())
	}
	press("/")
	if !m.cmSearching {
		t.Fatal("/ should open the prompt")
	}
	// Typing filters as it goes; a hit in a later comment of the thread
	// makes that comment the one the list shows.
	typ("style")
	if ids() != "T1" || m.cmIdx != 0 {
		t.Fatalf("threads = %s idx=%d", ids(), m.cmIdx)
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Comments (1 of 2 match “style”)", "/style", "@bob", "Style."} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Why the parens?") {
		t.Fatalf("the matching comment should replace the first one:\n%s", plain)
	}
	// backspace and ctrl+u edit the query.
	press("backspace")
	if m.cmSearchInput != "styl" || ids() != "T1" {
		t.Fatalf("backspace: %q %s", m.cmSearchInput, ids())
	}
	press("ctrl+u")
	if m.cmSearchInput != "" || ids() != "T1,T3" {
		t.Fatalf("ctrl+u should clear the filter: %q %s", m.cmSearchInput, ids())
	}
	// A query with no hits says so and leaves the list empty.
	typ("zzz")
	if len(m.cmThreads) != 0 {
		t.Fatalf("threads = %s", ids())
	}
	if plain = ansi.Strip(m.View().Content); !strings.Contains(plain, "No thread matches “zzz”") {
		t.Fatalf("empty state:\n%s", plain)
	}
	// esc in the prompt restores what was in force before it opened.
	press("esc")
	if m.cmSearching || m.cmQuery != "" || ids() != "T1,T3" {
		t.Fatalf("esc should cancel the prompt: q=%q %s", m.cmQuery, ids())
	}
	// The query also matches the file path and the comment authors, and
	// enter keeps it.
	press("/")
	typ("b.py")
	press("enter")
	if m.cmSearching || m.cmQuery != "b.py" || ids() != "T3" {
		t.Fatalf("path search: q=%q %s", m.cmQuery, ids())
	}
	if plain = ansi.Strip(m.View().Content); !strings.Contains(plain, "/b.py  1 of 2 thread(s)") {
		t.Fatalf("the bar should show the active filter:\n%s", plain)
	}
	press("/")
	press("ctrl+u")
	typ("dan")
	press("enter")
	if ids() != "T3" {
		t.Fatalf("author search: %s", ids())
	}
	// Resolved threads stay out of the results until t shows them.
	press("/")
	press("ctrl+u")
	typ("carol")
	press("enter")
	if len(m.cmThreads) != 0 {
		t.Fatalf("resolved threads should stay hidden: %s", ids())
	}
	press("t")
	if ids() != "T2" {
		t.Fatalf("t should bring the resolved match in: %s", ids())
	}
	if plain = ansi.Strip(m.View().Content); !strings.Contains(plain, "Comments (1 of 3 match “carol”)") {
		t.Fatalf("title with resolved shown:\n%s", plain)
	}
	press("t")
	// esc clears the filter first and closes the screen only after that.
	press("esc")
	if m.screen != screenComments || m.cmQuery != "" || ids() != "T1,T3" {
		t.Fatalf("esc should clear the search: screen=%v q=%q", m.screen, m.cmQuery)
	}
	press("esc")
	if m.screen != screenDiff {
		t.Fatal("a second esc closes the screen")
	}
	// The filter survives a trip into the diff and back.
	press("i")
	press("/")
	typ("style")
	press("enter")
	press("l")
	if m.screen != screenDiff || m.currentRow().thread.ID != "T1" {
		t.Fatalf("l should open the match in the diff: screen=%v", m.screen)
	}
	press("h")
	if m.screen != screenComments || ids() != "T1" {
		t.Fatalf("the filter should still be on: %s", ids())
	}
	press("/")
	press("ctrl+u")
	if ids() != "T1,T3" || m.cmIdx != 0 {
		t.Fatalf("threads = %s idx=%d", ids(), m.cmIdx)
	}
	// down moves the selection without leaving the prompt.
	m.handleKey(key("down"))
	if !m.cmSearching || m.cmIdx != 1 {
		t.Fatalf("down in the prompt: idx=%d searching=%v", m.cmIdx, m.cmSearching)
	}
	// While the prompt is open the list sits one line lower, so a click on
	// the last line of the first thread still lands on that thread.
	m.cmScroll = 0
	m.Update(tea.MouseClickMsg{X: 3, Y: headerH + paneHeaderH + 1 + 3, Button: tea.MouseLeft})
	if m.cmIdx != 0 {
		t.Fatalf("click should select the first thread: idx=%d", m.cmIdx)
	}
	// esc puts back the filter that was in force when the prompt opened.
	press("esc")
	if m.cmSearching || m.cmQuery != "style" || ids() != "T1" {
		t.Fatalf("esc should restore the filter: q=%q %s", m.cmQuery, ids())
	}
}

func TestCommentsReply(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) {
		switch k {
		case "esc":
			m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		default:
			m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		}
	}
	press("i")
	if m.screen != screenComments || m.cmThreads[m.cmIdx].ID != "T1" {
		t.Fatalf("screen=%v idx=%d", m.screen, m.cmIdx)
	}
	// r opens the reply panel for the selected thread, under the list.
	press("r")
	if m.overlay != overlayInput || m.inKind != inputReply || m.inThread == nil || m.inThread.ID != "T1" {
		t.Fatalf("reply not opened: overlay=%v thread=%v", m.overlay, m.inThread)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Reply to @alice on main.go:4") || !strings.Contains(plain, "Comments (") {
		t.Fatalf("the panel should sit under the list:\n%s", plain)
	}
	// Keys go to the textarea, not the list: j types instead of moving.
	press("j")
	if m.cmIdx != 0 || m.ta.Value() != "j" {
		t.Fatalf("the input should take the keys: idx=%d value=%q", m.cmIdx, m.ta.Value())
	}
	// Clicks do not move the selection behind the panel either.
	m.Update(tea.MouseClickMsg{X: 3, Y: headerH + paneHeaderH + 4, Button: tea.MouseLeft})
	if m.cmIdx != 0 {
		t.Fatalf("clicks should be ignored while replying: idx=%d", m.cmIdx)
	}
	// esc cancels and leaves the list as it was.
	press("esc")
	if m.overlay != overlayNone || m.screen != screenComments || m.inThread != nil {
		t.Fatalf("esc should close the panel: overlay=%v screen=%v", m.overlay, m.screen)
	}
	press("j")
	if m.cmIdx != 1 {
		t.Fatalf("the list should take keys again: idx=%d", m.cmIdx)
	}
	// Submitting posts to the selected thread and keeps the screen.
	press("r")
	if m.inThread == nil || m.inThread.ID != "T3" {
		t.Fatalf("reply should target the selected thread: %v", m.inThread)
	}
	m.ta.SetValue("on it")
	cmd := m.submitInput()
	if cmd == nil || m.overlay != overlayNone || m.screen != screenComments || m.busy == "" {
		t.Fatalf("submit: cmd=%v overlay=%v screen=%v busy=%q", cmd != nil, m.overlay, m.screen, m.busy)
	}
	// A thread with no comments cannot be replied to.
	m.busy = ""
	m.Update(threadsMsg{threads: []gh.Thread{{ID: "T9", Path: "main.go", Line: 4, DiffSide: "RIGHT"}}})
	press("r")
	if m.overlay == overlayInput || !strings.Contains(m.status, "no comments") {
		t.Fatalf("expected a refusal, overlay=%v status=%q", m.overlay, m.status)
	}
}

func TestConcurrentRequests(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) {
		switch k {
		case "esc":
			m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		default:
			m.handleKey(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		}
	}
	press("i")
	press("t") // list the resolved thread too: T2, T1, T3
	if len(m.cmThreads) != 3 {
		t.Fatalf("threads = %d", len(m.cmThreads))
	}
	// Resolve two different threads without waiting for the first answer.
	m.cmIdx = 1 // T1
	press("x")
	m.cmIdx = 2 // T3
	press("x")
	if len(m.running) != 2 {
		t.Fatalf("both resolves should be in flight: %d", len(m.running))
	}
	if !m.threadWorking("T1") || !m.threadWorking("T3") || m.threadWorking("T2") {
		t.Fatal("the list should know which threads are waiting")
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Resolve thread + 1 more") || !strings.Contains(plain, "· sending") {
		t.Fatalf("the bar should count the requests and the rows should mark them:\n%s", plain)
	}
	// Reply to a third thread while those two are still out.
	m.cmIdx = 0 // T2
	press("r")
	if m.overlay != overlayInput || m.inThread.ID != "T2" {
		t.Fatalf("reply should open while the resolves run: overlay=%v", m.overlay)
	}
	m.ta.SetValue("thanks")
	if cmd := m.submitInput(); cmd == nil || len(m.running) != 3 {
		t.Fatalf("the reply should join the queue: running=%d", len(m.running))
	}
	// Each answer clears its own entry; the bar follows what is left.
	seqs := []int{m.running[0].seq, m.running[1].seq, m.running[2].seq}
	m.Update(actionMsg{seq: seqs[1], label: "Resolve thread", refresh: true})
	if len(m.running) != 2 || m.threadWorking("T3") {
		t.Fatalf("only the landed request should clear: running=%d", len(m.running))
	}
	if !strings.HasPrefix(m.busy, "Post reply") {
		t.Fatalf("the bar should show what is left: %q", m.busy)
	}
	// The refresh that follows the last answer is the one that reloads the
	// PR as well; earlier ones only re-read the threads.
	m.Update(actionMsg{seq: seqs[0], label: "Resolve thread", refresh: true})
	m.Update(actionMsg{seq: seqs[2], label: "Post reply", refresh: true})
	if len(m.running) != 0 || m.busy != "Refreshing…" {
		t.Fatalf("running=%d busy=%q", len(m.running), m.busy)
	}
	// A slow answer to an old fetch must not undo what a newer one showed.
	fresh := append([]gh.Thread{}, m.threads...)
	fresh[0].IsResolved = true
	m.Update(threadsMsg{seq: 50, threads: fresh})
	m.Update(threadsMsg{seq: 20, threads: m.threads[:1]})
	if len(m.threads) != 3 || !m.threads[0].IsResolved {
		t.Fatalf("a stale reply should be dropped: %d threads", len(m.threads))
	}
}
