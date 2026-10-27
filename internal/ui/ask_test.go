package ui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/claude"
	"ghpr/internal/diff"
	"ghpr/internal/state"
)

// askCall is one stubbed call to the claude CLI.
type askCall struct{ bin, model, session, dir, prompt string }

// stubClaude makes the claude CLI available (with no checkout) and answers
// every question with answer (session "s1"), recording the calls.
func stubClaude(t *testing.T, answer string, err error) *[]askCall {
	t.Helper()
	calls := new([]askCall)
	prevAsk, prevAvail, prevDir := claudeAsk, claudeAvailable, askCheckout
	claudeAvailable = func(string) bool { return true }
	askCheckout = func(string) (string, string) { return "", "" }
	claudeAsk = func(bin, model, sid, dir, prompt string) (claude.Answer, error) {
		*calls = append(*calls, askCall{bin, model, sid, dir, prompt})
		return claude.Answer{Text: answer, SessionID: "s1"}, err
	}
	t.Cleanup(func() { claudeAsk, claudeAvailable, askCheckout = prevAsk, prevAvail, prevDir })
	return calls
}

// deliver runs cmd's leaf commands and feeds any claudeMsg back to Update.
func deliver(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			deliver(m, c)
		}
	case claudeMsg:
		m.Update(msg)
	}
}

// ask types question into the open input and submits it.
func ask(t *testing.T, m *Model, question string) {
	t.Helper()
	if m.overlay != overlayInput || m.inKind != inputAsk {
		t.Fatalf("question input should be open: overlay=%v kind=%v", m.overlay, m.inKind)
	}
	m.ta.SetValue(question)
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if strings.TrimSpace(question) != "" { // a refusal is only a status (and its 5s clear timer)
		deliver(m, cmd)
	}
}

func TestAskClaudeAboutSelection(t *testing.T) {
	m := newTestModel(t)
	m.SetClaude("", "sonnet")
	m.pr.Body = "Switches to a grouped import."
	calls := stubClaude(t, "It groups the import.\nNothing else changes.", nil)

	// rows: 5 `+import (`, 6 `+"fmt"`.
	m.cursor = 5
	m.toggleSelection()
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "Ask Claude about main.go:3-4") {
		t.Fatalf("input should name the lines:\n%s", plain)
	}
	ask(t, m, "why the parens?")

	if len(*calls) != 1 {
		t.Fatalf("calls = %d", len(*calls))
	}
	c := (*calls)[0]
	if c.bin != claude.DefaultBin || c.model != "sonnet" || c.session != "" || c.dir != "" {
		t.Fatalf("call = %+v", c)
	}
	for _, want := range []string{
		"Repository o/r, pull request #7: Test PR",
		"Switches to a grouped import.",
		"File: main.go (modified)",
		"Hunk: @@ -1,6 +1,8 @@",
		`     3 - import "fmt"`, // context row, not selected
		">    3 + import (",
		">    4 +     \"fmt\"", // tabs expanded, as on screen
		"Question: why the parens?",
	} {
		if !strings.Contains(c.prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, c.prompt)
		}
	}

	// The answer opens below the diff, which stays visible and shrinks.
	if m.overlay != overlayAnswer {
		t.Fatalf("answer panel should open, overlay=%v", m.overlay)
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Claude · main.go:3-4", "You", "┃ why the parens?", "Claude", "   It groups the import.", "package main", "follow-up"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q:\n%s", want, plain)
		}
	}
	if answer, _ := m.belowDiff(); answer == 0 || m.viewportH() != m.mainHeight()-2-answer {
		t.Fatalf("diff viewport should leave room for the panel: %d", m.viewportH())
	}
	// Keys go to the panel, not the diff.
	cur := m.cursor
	m.handleKeyCmd(t, 'j')
	if m.cursor != cur {
		t.Fatal("j should scroll the answer, not move the diff cursor")
	}

	// A follow-up resumes the session and sends only the question.
	m.handleKeyCmd(t, 'p')
	ask(t, m, "and the tests?")
	if c := (*calls)[1]; c.session != "s1" || c.prompt != "and the tests?" {
		t.Fatalf("follow-up call = %+v", c)
	}
	if len(m.chats) != 1 || len(m.activeChat().Entries) != 2 || m.overlay != overlayAnswer || len(m.asking) != 0 {
		t.Fatalf("follow-up should extend the conversation: %+v overlay=%v", m.chats, m.overlay)
	}

	// esc closes; a new p on other lines starts a second conversation.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != overlayNone || m.viewportH() != m.mainHeight()-2 {
		t.Fatal("esc should close the panel and give the diff its room back")
	}
	m.cursor = 11
	m.handleKeyCmd(t, 'p')
	ask(t, m, "what prints?")
	if c := (*calls)[2]; c.session != "" || !strings.Contains(c.prompt, "File: main.go") {
		t.Fatalf("new conversation should not resume: %+v", c)
	}
	if len(m.chats) != 2 || m.activeChat().Loc != "main.go:8" {
		t.Fatalf("new conversation should be added: %+v", m.chats)
	}
}

func TestChatsScreen(t *testing.T) {
	m := newTestModel(t)
	stubClaude(t, "An answer.", nil)
	m.handleKeyCmd(t, 'P')
	if m.screen != screenDiff || !strings.Contains(m.status, "No conversations with Claude") {
		t.Fatalf("P with no conversations: screen=%v status=%q", m.screen, m.status)
	}
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "first?")
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.selectFile(1) // b.py
	m.cursor = 3    // +    return 2
	m.handleKeyCmd(t, 'p')
	ask(t, m, "second?")
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	// P lists both, the latest selected.
	m.handleKeyCmd(t, 'P')
	if m.screen != screenChats || m.chIdx != 1 {
		t.Fatalf("P should list the conversations: screen=%v idx=%d", m.screen, m.chIdx)
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Claude (2)", "main.go:4", "? first?", "b.py:2", "An answer."} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list missing %q:\n%s", want, plain)
		}
	}
	// Opening the first jumps to its file and line with the panel open.
	m.handleKeyCmd(t, 'k')
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenDiff || m.overlay != overlayAnswer || m.fileIdx != 0 || m.activeChat().Loc != "main.go:4" {
		t.Fatalf("open: screen=%v overlay=%v file=%d", m.screen, m.overlay, m.fileIdx)
	}
	if _, n, _ := m.rows[m.cursor].nums(); n != 4 {
		t.Fatalf("cursor should be on line 4, got %d", n)
	}
	// Remove one from the list.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.handleKeyCmd(t, 'P')
	m.handleKeyCmd(t, 'd')
	if len(m.chats) != 1 || m.chats[0].Loc != "b.py:2" || m.chatID != "" {
		t.Fatalf("d should remove the selected conversation: %+v active=%q", m.chats, m.chatID)
	}
}

func TestChatsPersistAndContinue(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t)
	m.SetStore(st)
	calls := stubClaude(t, "Kept.", nil)
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "remember?")
	if got := st.GetChats(m.prKey()); len(got) != 1 || got[0].Session != "s1" || got[0].Entries[0].A != "Kept." {
		t.Fatalf("store should hold the conversation: %+v", got)
	}

	// A fresh model on the same store has it once the diff loads, and can
	// continue it.
	st2, _ := state.Open(dir)
	m2 := newTestModel(t)
	m2.SetStore(st2)
	m2.pending = 2
	m2.Update(diffMsg{files: diff.Parse(sample)})
	m2.handleKeyCmd(t, 'P')
	m2.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m2.overlay != overlayAnswer || !strings.Contains(ansi.Strip(m2.View().Content), "Kept.") {
		t.Fatalf("reopened conversation should show:\n%s", ansi.Strip(m2.View().Content))
	}
	m2.handleKeyCmd(t, 'p')
	ask(t, m2, "still there?")
	if c := (*calls)[1]; c.session != "s1" || c.prompt != "still there?" {
		t.Fatalf("a later follow-up should resume: %+v", c)
	}
	if got := st2.GetChats(m2.prKey()); len(got[0].Entries) != 2 {
		t.Fatalf("follow-up should be saved: %+v", got)
	}
}

func TestChatResumeFailsReplays(t *testing.T) {
	m := newTestModel(t)
	calls := stubClaude(t, "First.", nil)
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "one?")
	// The session cannot be resumed (ghpr started elsewhere): a new session
	// gets the context and the conversation so far.
	claudeAsk = func(bin, model, sid, dir, prompt string) (claude.Answer, error) {
		*calls = append(*calls, askCall{bin, model, sid, dir, prompt})
		if sid != "" {
			return claude.Answer{}, errors.New("No conversation found with session ID")
		}
		return claude.Answer{Text: "Replayed.", SessionID: "s2"}, nil
	}
	m.handleKeyCmd(t, 'p')
	ask(t, m, "two?")
	if len(*calls) != 3 {
		t.Fatalf("should try to resume, then replay: %+v", *calls)
	}
	r := (*calls)[2]
	for _, want := range []string{"File: main.go", "Earlier in this conversation:", "Q: one?\nA: First.", "Question: two?"} {
		if r.session != "" || !strings.Contains(r.prompt, want) {
			t.Fatalf("replay missing %q: %+v", want, r)
		}
	}
	c := m.activeChat()
	if c.Session != "s2" || len(c.Entries) != 2 || !strings.Contains(m.status, "new Claude session") {
		t.Fatalf("replay should continue the conversation: %+v status=%q", c, m.status)
	}
}

func TestAskClaudeCopyAndComment(t *testing.T) {
	m := newTestModel(t)
	stubClaude(t, "Use a grouped import.", nil)
	got := stubClipboard(t)

	m.cursor = 6 // +"fmt", new line 4
	m.handleKeyCmd(t, 'p')
	ask(t, m, "ok?")
	m.handleKeyCmd(t, 'y') // the platform clipboard is written right away
	if *got != "Use a grouped import." {
		t.Fatalf("clipboard = %q", *got)
	}
	m.handleKeyCmd(t, 'c')
	if m.overlay != overlayInput || m.inKind != inputComment {
		t.Fatalf("c should open a comment: overlay=%v kind=%v", m.overlay, m.inKind)
	}
	if m.ta.Value() != "Use a grouped import." || m.inComment.Line != 4 || m.inComment.Side != "RIGHT" || m.inComment.Path != "main.go" {
		t.Fatalf("comment draft = %q on %+v", m.ta.Value(), m.inComment)
	}
}

func TestAskClaudeErrors(t *testing.T) {
	m := newTestModel(t)
	stubClaude(t, "", errors.New("Credit balance is too low"))
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "ok?")
	if m.overlay == overlayAnswer || !m.statusErr || !strings.Contains(m.status, "Credit balance is too low") || len(m.chats) != 0 {
		t.Fatalf("error should show in the status bar: overlay=%v status=%q", m.overlay, m.status)
	}

	// An empty question is refused without calling Claude.
	calls := stubClaude(t, "x", nil)
	m.handleKeyCmd(t, 'p')
	ask(t, m, "  ")
	if len(*calls) != 0 || m.status != "Question is empty" {
		t.Fatalf("empty question: calls=%d status=%q", len(*calls), m.status)
	}

	// Without the CLI, p says so instead of opening the input.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	claudeAvailable = func(string) bool { return false }
	m.handleKeyCmd(t, 'p')
	if m.overlay == overlayInput || !strings.Contains(m.status, "claude not found") {
		t.Fatalf("missing CLI: overlay=%v status=%q", m.overlay, m.status)
	}
}

func TestAskClaudeAnswerWhileBusy(t *testing.T) {
	m := newTestModel(t)
	calls := stubClaude(t, "Later.", nil)
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	m.ta.SetValue("q")
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !strings.Contains(m.busy, "Asking Claude") {
		t.Fatalf("busy = %q", m.busy)
	}
	// Claude works in the background: the diff has the keys meanwhile.
	if m.overlay != overlayNone || !strings.Contains(m.busy, "Asking Claude about main.go:4") {
		t.Fatalf("submitting should return to the diff: overlay=%v busy=%q", m.overlay, m.busy)
	}
	m.handleKeyCmd(t, 'j')
	if m.cursor != 7 {
		t.Fatalf("j should move the diff cursor while Claude works, cursor=%d", m.cursor)
	}
	// P lists the conversation with its question waiting, and opening it
	// shows a thinking line; a follow-up has to wait for the answer.
	m.handleKeyCmd(t, 'P')
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "? q") || !strings.Contains(plain, "Claude is thinking…") {
		t.Fatalf("the waiting question should be listed:\n%s", plain)
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if plain := ansi.Strip(m.View().Content); m.overlay != overlayAnswer || !strings.Contains(plain, "┃ q") || !strings.Contains(plain, "thinking…") || !strings.Contains(plain, "Claude · main.go:4") {
		t.Fatalf("the waiting question should show:\n%s", plain)
	}
	m.handleKeyCmd(t, 'p')
	if !strings.Contains(m.status, "still answering") || m.overlay != overlayAnswer {
		t.Fatalf("p while waiting: %q", m.status)
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	// Another question on other lines goes out alongside the first.
	m.cursor = 11
	m.handleKeyCmd(t, 'p')
	m.ta.SetValue("again")
	_, cmd2 := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if len(m.asking) != 2 || len(m.chats) != 2 || !strings.Contains(m.busy, "+ 1 more") {
		t.Fatalf("second question should run too: asking=%d busy=%q", len(m.asking), m.busy)
	}
	// The first answer arrives while a comment is being typed: it waits behind P.
	m.cursor = 6
	m.handleKeyCmd(t, 'c')
	deliver(m, cmd)
	if m.overlay != overlayInput || !strings.Contains(m.status, "Claude answered about main.go:4") || len(m.asking) != 1 {
		t.Fatalf("answer should not take over the input: overlay=%v status=%q", m.overlay, m.status)
	}
	// The second lands on the idle diff and opens there.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	deliver(m, cmd2)
	if m.overlay != overlayAnswer || m.activeChat().Loc != "main.go:8" || m.busy != "" || len(*calls) != 2 {
		t.Fatalf("second answer should open: overlay=%v loc=%q busy=%q", m.overlay, m.activeChat().Loc, m.busy)
	}
	// Leaving the PR drops an answer still on its way.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.handleKeyCmd(t, 'p')
	m.ta.SetValue("q2")
	_, cmd = m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m.resetAsk()
	deliver(m, cmd)
	if len(m.chats) != 0 {
		t.Fatal("an answer for a PR that was left should be ignored")
	}
}

func TestAskClaudeInCheckout(t *testing.T) {
	m := newTestModel(t)
	calls := stubClaude(t, "ok", nil)
	askCheckout = func(head string) (string, string) {
		if head != "abc" {
			t.Fatalf("checkout should be matched against the PR head, got %q", head)
		}
		return "/src/repo", ""
	}

	m.cursor = 5
	m.toggleSelection()
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "why?")
	c := (*calls)[0]
	if c.dir != "/src/repo" {
		t.Fatalf("claude should run in the checkout: %+v", c)
	}
	for _, want := range []string{"checkout of this pull request's head branch (feat)", "Read main.go", "   3 + import (", "   4 +     \"fmt\""} {
		if !strings.Contains(c.prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, c.prompt)
		}
	}
	// Claude reads the surrounding code itself, so only the selection goes.
	if strings.Contains(c.prompt, "package main") || strings.Contains(c.prompt, `import "fmt"`) {
		t.Fatalf("lines outside the selection should be left to Claude:\n%s", c.prompt)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "reads the checkout") {
		t.Fatal("the panel should say Claude reads the checkout")
	}
	// Follow-ups resume in the same checkout.
	m.handleKeyCmd(t, 'p')
	ask(t, m, "more?")
	if c := (*calls)[1]; c.dir != "/src/repo" || c.session != "s1" {
		t.Fatalf("follow-up = %+v", c)
	}
	// Once the checkout has moved off the PR head, the conversation carries
	// on without file access, from the diff-only context.
	askCheckout = func(string) (string, string) { return "", "moved" }
	m.handleKeyCmd(t, 'p')
	ask(t, m, "now?")
	r := (*calls)[2]
	if r.dir != "" || r.session != "" || !strings.Contains(r.prompt, `     3 - import "fmt"`) || !strings.Contains(r.prompt, "Q: more?") {
		t.Fatalf("moved checkout should replay without files: %+v", r)
	}
	if m.activeChat().Dir != "" {
		t.Fatal("the conversation should no longer claim the checkout")
	}
}

func TestAskClaudeCheckoutMismatchSaysWhy(t *testing.T) {
	m := newTestModel(t)
	calls := stubClaude(t, "ok", nil)
	askCheckout = func(string) (string, string) { return "", "the checkout is at 1234567, not the PR head abc" }
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	if !strings.Contains(ansi.Strip(m.View().Content), "no file access: the checkout is at 1234567") {
		t.Fatalf("input title should say why Claude cannot read the files:\n%s", ansi.Strip(m.View().Content))
	}
	ask(t, m, "q")
	if c := (*calls)[0]; c.dir != "" || !strings.Contains(c.prompt, `     3 - import "fmt"`) {
		t.Fatalf("mismatch should use the diff-only context: %+v", c)
	}
}

func TestCheckoutFor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("commit", "-q", "--allow-empty", "-m", "x")
	head := run("rev-parse", "HEAD")
	wd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	cwd, _ := os.Getwd()

	t.Setenv("NVIM_SOCK", "")
	if d, why := checkoutFor(head); d != "" || why != "" {
		t.Fatalf("without $NVIM_SOCK there is no checkout: %q %q", d, why)
	}
	t.Setenv("NVIM_SOCK", "/tmp/nvim.sock")
	if d, why := checkoutFor(head); d != cwd || why != "" {
		t.Fatalf("at the PR head the working directory is the checkout: %q %q", d, why)
	}
	if d, why := checkoutFor("0123456789abcdef"); d != "" || !strings.Contains(why, "not the PR head 0123456") {
		t.Fatalf("another commit is not the checkout: %q %q", d, why)
	}
}

func TestFollowUpKeepsConversationVisible(t *testing.T) {
	m := newTestModel(t)
	stubClaude(t, "It groups the import.", nil)
	m.cursor = 6
	m.handleKeyCmd(t, 'p')
	ask(t, m, "why the parens?")

	m.handleKeyCmd(t, 'p') // follow-up from the panel
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Claude · main.go:4", "It groups the import.", "Follow-up for Claude about main.go:4", `4 +     "fmt"`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("diff, conversation and input should all show while typing; missing %q:\n%s", want, plain)
		}
	}
	answer, input := m.belowDiff()
	if answer == 0 || input != inputPanelH || m.viewportH() != m.mainHeight()-2-answer-input || m.mainHeight()-answer-input < minDiffRows {
		t.Fatalf("layout: answer=%d input=%d viewport=%d main=%d", answer, input, m.viewportH(), m.mainHeight())
	}
	// Once sent the diff gets the keys back; reopened, the conversation
	// shows the earlier turns with the question waiting.
	m.ta.SetValue("and os?")
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.overlay != overlayNone {
		t.Fatalf("a sent follow-up should return to the diff, overlay=%v", m.overlay)
	}
	m.handleKeyCmd(t, 'P')
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	plain = ansi.Strip(m.View().Content)
	for _, want := range []string{"It groups the import.", "┃ and os?", "thinking…"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("waiting follow-up should keep the conversation; missing %q:\n%s", want, plain)
		}
	}
	deliver(m, cmd)
	if len(m.asking) != 0 || len(m.activeChat().Entries) != 2 || m.overlay != overlayAnswer {
		t.Fatal("the answer should replace the thinking line")
	}
	m.handleKeyCmd(t, 'p')
	// A new question (not a follow-up) has no conversation to show yet.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}) // back to the panel
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}) // close it
	m.handleKeyCmd(t, 'p')
	if answer, _ := m.belowDiff(); answer != 0 {
		t.Fatalf("a new question should show only the input, answer=%d", answer)
	}
	// On a short terminal the input alone is shown.
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	m.handleKeyCmd(t, 'P')
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.handleKeyCmd(t, 'p')
	if answer, input := m.belowDiff(); answer != 0 || input != inputPanelH {
		t.Fatalf("short terminal: answer=%d input=%d", answer, input)
	}
}

func TestInputHighlightsCursorLine(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 6
	m.handleKeyCmd(t, 'c')
	m.ta.SetValue("first line\nsecond line") // the cursor ends on the second line
	var first, second string
	for _, l := range strings.Split(m.View().Content, "\n") {
		switch {
		case strings.Contains(l, "first line"):
			first = l
		case strings.Contains(l, "second line"):
			second = l
		}
	}
	bg := "\x1b[48;"
	if !strings.Contains(second, bg) || strings.Contains(first, bg) {
		t.Fatalf("only the cursor line should be highlighted:\nfirst  %q\nsecond %q", first, second)
	}
}
