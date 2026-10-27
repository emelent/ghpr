package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/claude"
	"ghpr/internal/diff"
	"ghpr/internal/editor"
	"ghpr/internal/gh"
	"ghpr/internal/keys"
	"ghpr/internal/state"
)

// ---------- asking Claude about lines (p), conversations (P) ----------

// Claude CLI and checkout detection; tests replace these.
var (
	claudeAsk       = claude.Ask
	claudeAvailable = claude.Available
	askCheckout     = checkoutFor
)

// checkoutFor returns the directory holding the code under review at commit
// head, or "" and why not. $NVIM_SOCK being set means ghpr runs in the
// checkout the Neovim hand-off (o) edits; it only counts when that checkout
// is at the PR's head commit, otherwise Claude would read other code.
func checkoutFor(head string) (dir, why string) {
	if os.Getenv(editor.EnvSock) == "" {
		return "", ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	out, err := exec.Command("git", "-C", wd, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "the current directory is not a git checkout"
	}
	sha := strings.TrimSpace(string(out))
	if head == "" || sha != head {
		return "", fmt.Sprintf("the checkout is at %s, not the PR head %s", short(sha), short(head))
	}
	return wd, ""
}

func short(sha string) string { return sha[:min(7, len(sha))] }

// askContextRows is how many rows either side of the selection go into the
// first question when Claude cannot read the checkout.
const askContextRows = 10

// askBodyLimit caps how much of the PR description goes into the prompt.
const askBodyLimit = 2000

// asking is a question Claude is still answering. Questions run in the
// background, so several can be out at once, but only one per
// conversation: a follow-up needs the session the previous answer ends.
type asking struct {
	seq    int    // its running entry
	chatID string // the conversation it belongs to
	q      string
}

// claudeMsg carries Claude's answer back to Update.
type claudeMsg struct {
	seq    int    // the running entry to finish
	gen    int    // askGen when asked; a newer one means the PR changed since
	chatID string // the conversation asked in
	q      string
	dir    string // where Claude ran; changes when a follow-up had to start over
	replay bool   // the old session could not be resumed
	ans    claude.Answer
	err    error
}

// SetClaude sets the claude binary (default "claude") and model (default:
// the CLI's own) used to answer questions about the selection.
func (m *Model) SetClaude(bin, model string) {
	if bin == "" {
		bin = claude.DefaultBin
	}
	m.claudeBin, m.claudeModel = bin, model
}

// activeChat is the conversation shown in the answer panel, or nil.
func (m *Model) activeChat() *state.Chat {
	for i := range m.chats {
		if m.chats[i].ID == m.chatID {
			return &m.chats[i]
		}
	}
	return nil
}

// waitingIn is the question Claude is still answering in conversation id,
// or nil.
func (m *Model) waitingIn(id string) *asking {
	for i := range m.asking {
		if m.asking[i].chatID == id {
			return &m.asking[i]
		}
	}
	return nil
}

func (m *Model) headSHA() string {
	if m.pr == nil {
		return ""
	}
	return m.pr.HeadRefOid
}

// startAsk opens the question input for the selection, or the row under the
// cursor, starting a new conversation.
func (m *Model) startAsk() tea.Cmd {
	if len(m.files) == 0 || len(m.rows) == 0 {
		return m.setStatus("No code to ask about", true)
	}
	if m.claudeBin == "" {
		m.claudeBin = claude.DefaultBin
	}
	if !claudeAvailable(m.claudeBin) {
		return m.setStatus(m.claudeBin+" not found (install Claude Code or set --claude)", true)
	}
	lo, hi, ok := m.selection()
	if !ok {
		lo, hi = m.cursor, m.cursor
	}
	dir, why := askCheckout(m.headSHA())
	c := state.Chat{Path: m.files[m.fileIdx].Path(), Loc: m.askLocation(lo, hi), Dir: dir, Context: m.askContext(lo, hi, dir != "")}
	if dir != "" {
		c.Fallback = m.askContext(lo, hi, false)
	}
	for i := hi; i >= lo; i-- { // the line to come back to
		if o, n, ok := m.rows[i].nums(); ok {
			c.Line, c.Side = n, "RIGHT"
			if n == 0 {
				c.Line, c.Side = o, "LEFT"
			}
			break
		}
	}
	// Remember where a comment would go, for turning an answer into one.
	lc, _, errMsg := m.commentTarget()
	c.Comment = state.ChatComment{Line: lc.Line, Side: lc.Side, StartLine: lc.StartLine, StartSide: lc.StartSide}
	c.CommentErr = errMsg
	m.askDraft = c
	m.askFollow = false
	title := "Ask Claude about " + c.Loc
	if why != "" {
		title += " (no file access: " + why + ")"
	}
	m.openInput(inputAsk, title)
	return nil
}

// startFollowUp opens the question input to continue the active
// conversation.
func (m *Model) startFollowUp() {
	c := m.activeChat()
	if c == nil {
		return
	}
	m.askFollow = true
	m.askScroll = -1 // the conversation stays above the input, at the answer being replied to
	m.openInput(inputAsk, "Follow-up for Claude about "+c.Loc)
}

// submitAsk sends question to Claude in the background and returns to the
// diff, which stays usable meanwhile; the answer opens below it when it
// lands. A new conversation joins the list (P) right away, with the question
// waiting in it, but is only saved once answered. A follow-up resumes
// the conversation's Claude session in the directory it ran in; when that
// fails (ghpr started elsewhere, session gone) or the checkout moved off the
// PR head, a new session gets the context and the transcript replayed.
func (m *Model) submitAsk(question string) tea.Cmd {
	var c state.Chat
	if m.askFollow {
		ac := m.activeChat()
		if ac == nil {
			return m.setStatus("That conversation is gone", true)
		}
		c = *ac
	} else {
		c = m.askDraft
		now := time.Now()
		c.ID = strconv.FormatInt(now.UnixNano(), 36)
		c.CreatedAt, c.UpdatedAt = now, now
	}
	seq, ok := m.startInflight("Asking Claude about "+c.Loc, "ask:"+c.ID, "")
	if !ok {
		return m.setStatus("Claude is still answering in this conversation", true)
	}
	if !m.askFollow {
		m.chats = append(m.chats, c)
	}
	m.asking = append(m.asking, asking{seq: seq, chatID: c.ID, q: question})
	m.chatID = c.ID
	m.overlay = overlayNone
	bin, model, gen, id := m.claudeBin, m.claudeModel, m.askGen, c.ID
	if !m.askFollow {
		prompt := c.Context + "\n\nQuestion: " + question
		return tea.Batch(m.tick(), func() tea.Msg {
			ans, err := claudeAsk(bin, model, "", c.Dir, prompt)
			return claudeMsg{seq: seq, gen: gen, chatID: id, q: question, dir: c.Dir, ans: ans, err: err}
		})
	}
	cur, _ := askCheckout(m.headSHA())
	resume := c.Session != "" && (c.Dir == "" || c.Dir == cur)
	replayDir, ctx := "", c.Context
	if c.Dir != "" {
		if c.Dir == cur {
			replayDir = c.Dir
		} else if c.Fallback != "" {
			ctx = c.Fallback
		}
	}
	replay := ctx + "\n\nEarlier in this conversation:\n" + transcript(c.Entries) + "\nQuestion: " + question
	return tea.Batch(m.tick(), func() tea.Msg {
		if resume {
			if ans, err := claudeAsk(bin, model, c.Session, c.Dir, question); err == nil {
				return claudeMsg{seq: seq, gen: gen, chatID: id, q: question, dir: c.Dir, ans: ans}
			}
		}
		ans, err := claudeAsk(bin, model, "", replayDir, replay)
		return claudeMsg{seq: seq, gen: gen, chatID: id, q: question, dir: replayDir, replay: true, ans: ans, err: err}
	})
}

// transcript renders earlier questions and answers for a replayed session.
func transcript(entries []state.ChatEntry) string {
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("Q: " + e.Q + "\nA: " + e.A + "\n\n")
	}
	return sb.String()
}

// gotAnswer records Claude's answer in its conversation, saves it, and shows
// it: in the panel below the diff when that conversation is already open
// there or nothing else is, otherwise it waits behind P.
func (m *Model) gotAnswer(msg claudeMsg) tea.Cmd {
	m.finishAction(msg.seq)
	if msg.gen != m.askGen {
		return nil // asked about another PR
	}
	for i := range m.asking {
		if m.asking[i].seq == msg.seq {
			m.asking = append(m.asking[:i], m.asking[i+1:]...)
			break
		}
	}
	ci := -1
	for i := range m.chats {
		if m.chats[i].ID == msg.chatID {
			ci = i
		}
	}
	if msg.err != nil {
		if ci >= 0 && len(m.chats[ci].Entries) == 0 { // the new conversation never started
			m.removeChat(ci)
		}
		return m.setStatus("Claude: "+msg.err.Error(), true)
	}
	if ci < 0 {
		return m.setStatus("Claude answered in a conversation that was removed", true)
	}
	now := time.Now()
	c := &m.chats[ci]
	c.Entries = append(c.Entries, state.ChatEntry{Q: msg.q, A: msg.ans.Text, At: now})
	c.Session, c.Dir, c.UpdatedAt = msg.ans.SessionID, msg.dir, now
	cmds := []tea.Cmd{m.saveChats()}
	if msg.replay {
		cmds = append(cmds, m.setStatus("Started a new Claude session with the earlier conversation", false))
	}
	switch {
	case m.screen == screenDiff && m.overlay == overlayAnswer && m.chatID == c.ID:
		m.askScroll = -1 // show the start of the new answer
	case m.screen == screenDiff && m.overlay == overlayNone:
		m.chatID = c.ID
		m.overlay = overlayAnswer
		m.askScroll = -1
	default:
		cmds = append(cmds, m.setStatus("Claude answered about "+c.Loc+" · "+m.keys.First(keys.Diff, "chats")+" lists the conversations", false))
	}
	return tea.Batch(cmds...)
}

// removeChat drops conversation i, closing the panel when it showed it.
func (m *Model) removeChat(i int) {
	if m.chats[i].ID == m.chatID {
		m.chatID = ""
		if m.overlay == overlayAnswer {
			m.overlay = overlayNone
		}
	}
	m.chats = append(m.chats[:i], m.chats[i+1:]...)
	m.chIdx = min(m.chIdx, max(0, len(m.chats)-1))
}

// loadChats restores the PR's conversations from the store.
func (m *Model) loadChats() {
	if m.store == nil {
		return
	}
	pending := m.chats
	m.chats = m.store.GetChats(m.prKey())
	for _, c := range pending { // new conversations still waiting on their first answer
		if len(c.Entries) == 0 && m.waitingIn(c.ID) != nil {
			m.chats = append(m.chats, c)
		}
	}
	if m.chIdx >= len(m.chats) {
		m.chIdx = 0
	}
}

// saveChats writes the PR's conversations to the store.
func (m *Model) saveChats() tea.Cmd {
	if m.store == nil {
		return nil
	}
	var saved []state.Chat
	for _, c := range m.chats {
		if len(c.Entries) > 0 { // not ones still waiting on their first answer
			saved = append(saved, c)
		}
	}
	m.store.SetChats(m.prKey(), saved)
	if err := m.store.Save(); err != nil {
		return m.setStatus("Save Claude conversations: "+err.Error(), true)
	}
	return nil
}

// resetAsk forgets the PR's conversations in memory when leaving it; answers
// still on their way are dropped. The store keeps them.
func (m *Model) resetAsk() {
	m.askGen++
	m.asking = nil
	m.chats, m.chatID = nil, ""
	m.askScroll, m.chIdx, m.chScroll = 0, 0, 0
}

// handleAnswerKey handles keys while the answer panel has focus.
func (m *Model) handleAnswerKey(k string) (tea.Model, tea.Cmd) {
	c := m.activeChat()
	if c == nil {
		m.overlay = overlayNone
		return m, nil
	}
	answer, _ := m.belowDiff()
	page := max(1, answer-2)
	switch k {
	case "esc", "q":
		m.overlay = overlayNone
	case "j", "down":
		m.askScroll++
	case "k", "up":
		m.askScroll--
	case "ctrl+d", "pgdown", "space":
		m.askScroll += page / 2
	case "ctrl+u", "pgup":
		m.askScroll -= page / 2
	case "g", "home":
		m.askScroll = 0
	case "G", "end":
		m.askScroll = 1 << 30 // clamped when rendered
	case "p":
		if m.waitingIn(c.ID) != nil {
			return m, m.setStatus("Claude is still answering", true)
		}
		m.startFollowUp()
	case "y":
		a := lastAnswer(c)
		if a == "" {
			return m, m.setStatus("No answer to copy yet", true)
		}
		return m, tea.Batch(copyToClipboard(a), m.setStatus(fmt.Sprintf("Copied Claude's answer (%d line(s))", strings.Count(a, "\n")+1), false))
	case "c":
		if c.CommentErr != "" {
			return m, m.setStatus(c.CommentErr, true)
		}
		if m.pr == nil {
			return m, nil
		}
		if lastAnswer(c) == "" {
			return m, m.setStatus("No answer to comment with yet", true)
		}
		m.inComment = gh.LineComment{Path: c.Path, Line: c.Comment.Line, Side: c.Comment.Side, StartLine: c.Comment.StartLine, StartSide: c.Comment.StartSide}
		m.openInput(inputComment, "New comment on "+c.Loc)
		m.ta.SetValue(lastAnswer(c))
	}
	if m.askScroll < 0 {
		m.askScroll = 0
	}
	return m, nil
}

func lastAnswer(c *state.Chat) string {
	if len(c.Entries) == 0 {
		return ""
	}
	return c.Entries[len(c.Entries)-1].A
}

// answerH is the height of the answer panel below the diff.
func (m *Model) answerH() int { return max(inputPanelH, m.mainHeight()/2) }

// minDiffRows is how much of the diff stays visible (title and border
// included) while the conversation and the follow-up input share the pane.
const minDiffRows = 6

// belowDiff gives the heights of the panels under the diff: the answer
// panel and the text input. The answer panel is no taller than the
// conversation needs. While a follow-up is typed the conversation stays
// visible above the input, shrunk to leave the diff minDiffRows.
func (m *Model) belowDiff() (answer, input int) {
	fit := func(h int) int {
		body, _ := m.answerBody(m.diffWidth())
		return min(h, max(4, len(body)+2)) // border and title
	}
	switch m.overlay {
	case overlayAnswer:
		return fit(m.answerH()), 0
	case overlayInput:
		if m.inKind == inputAsk && m.askFollow && m.activeChat() != nil {
			answer = fit(min(m.answerH(), m.mainHeight()-inputPanelH-minDiffRows))
			if answer < 4 { // too short to be useful: the input alone
				answer = 0
			}
		}
		return answer, inputPanelH
	}
	return 0, 0
}

// answerBody wraps the shown conversation to width, turn by turn: a "You"
// label over each question, marked by a bar in the margin, then a "Claude"
// label over the indented answer (code fences dimmed). A question still
// waiting ends with a thinking line. last is the first line of the newest
// turn.
func (m *Model) answerBody(width int) (body []string, last int) {
	c := m.activeChat()
	if c == nil {
		return nil, 0
	}
	you := lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	claudeSty := lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	qText := lipgloss.NewStyle().Foreground(colText).Bold(true)
	w := max(8, width-4)
	question := func(q string) {
		if len(body) > 0 {
			body = append(body, "")
		}
		last = len(body)
		body = append(body, " "+you.Render("You"))
		for _, l := range strings.Split(ansi.Wrap(q, w, ""), "\n") {
			body = append(body, " "+you.Render("┃")+" "+qText.Render(l))
		}
		body = append(body, " "+claudeSty.Render("Claude"))
	}
	for _, e := range c.Entries {
		question(e.Q)
		fence := false
		for _, l := range strings.Split(ansi.Wrap(expandTabs(e.A), w, ""), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				fence = !fence
				continue
			}
			if fence {
				l = styDim.Render(l)
			}
			body = append(body, "   "+l)
		}
	}
	if a := m.waitingIn(c.ID); a != nil {
		question(a.q)
		body = append(body, "   "+styAccent.Render(ansi.Strip(m.spinner.View()))+styDim.Render("thinking…"))
	}
	return body, last
}

// renderAnswer renders the active conversation: a border, the title and the
// questions and answers wrapped to the pane, scrolled by askScroll.
func (m *Model) renderAnswer(width, height int) []string {
	c := m.activeChat()
	if c == nil {
		c = &state.Chat{}
	}
	body, last := m.answerBody(width)
	avail := max(1, height-2)
	if m.askScroll < 0 {
		m.askScroll = last
	}
	m.askScroll = max(0, min(m.askScroll, len(body)-avail))
	info := ""
	if c.Dir != "" {
		info = "  reads the checkout"
	}
	if len(body) > avail {
		info += fmt.Sprintf("  %d-%d/%d", m.askScroll+1, min(m.askScroll+avail, len(body)), len(body))
	}
	title := styInputTtl.Render(" Claude · "+c.Loc) + styDim.Render(info)
	lines := []string{styBorder.Render(strings.Repeat("─", width)), padRight(truncateTail(title, width), width)}
	for i := m.askScroll; i < len(body) && len(lines) < height; i++ {
		lines = append(lines, padRight(truncateTail(body[i], width), width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}

// askLocation describes rows lo..hi for titles: "main.go:4-6".
func (m *Model) askLocation(lo, hi int) string {
	path := m.files[m.fileIdx].Path()
	first, last := 0, 0
	for i := lo; i <= hi && i < len(m.rows); i++ {
		if n := m.rowLineNum(i); n > 0 {
			if first == 0 {
				first = n
			}
			last = n
		}
	}
	switch {
	case first == 0:
		return path
	case first == last:
		return fmt.Sprintf("%s:%d", path, first)
	}
	return fmt.Sprintf("%s:%d-%d", path, first, last)
}

// rowLineNum is the line number shown for row i: the new side, or the old
// side for a removed line; 0 for rows that are not code.
func (m *Model) rowLineNum(i int) int {
	o, n, ok := m.rows[i].nums()
	if !ok {
		return 0
	}
	if n > 0 {
		return n
	}
	return o
}

// askContext is the start of a new conversation: the PR, the file and the
// selected rows lo..hi. Unlike rowsText it keeps the +/- markers and line
// numbers, which Claude needs to tell the change from the code around it.
// In a checkout Claude reads the surrounding code itself, so only the
// selection is sent; otherwise askContextRows either side go along, with the
// selected lines marked by ">".
func (m *Model) askContext(lo, hi int, checkout bool) string {
	var sb strings.Builder
	if m.pr != nil {
		fmt.Fprintf(&sb, "Repository %s, pull request #%d: %s\n", m.client.Repo, m.pr.Number, m.pr.Title)
		if body := strings.TrimSpace(m.pr.Body); body != "" {
			if len(body) > askBodyLimit {
				body = body[:askBodyLimit] + "…"
			}
			sb.WriteString("\nPR description:\n" + body + "\n")
		}
	}
	f := &m.files[m.fileIdx]
	fmt.Fprintf(&sb, "\nFile: %s (%s)\n", f.Path(), f.Status)
	around := askContextRows
	if checkout {
		around = 0
		head := ""
		if m.pr != nil {
			head = " (" + m.pr.HeadRefName + ")"
		}
		fmt.Fprintf(&sb, "The working directory is a checkout of this pull request's head branch%s. "+
			"Read %s and any other files you need for context instead of relying on the lines below alone.\n", head, f.Path())
	}
	for i := min(lo, len(m.rows)-1); i >= 0; i-- {
		if m.rows[i].kind == rowHunk {
			sb.WriteString("Hunk: " + m.rows[i].hunk.Header + "\n")
			break
		}
	}
	if around > 0 {
		sb.WriteString("\nCode (line number, then + added / - removed / space unchanged; the lines asked about start with >):\n")
	} else {
		sb.WriteString("\nThe lines asked about, from the diff (line number, then + added / - removed / space unchanged):\n")
	}
	var threads []*gh.Thread
	for i := max(0, lo-around); i <= hi+around && i < len(m.rows); i++ {
		mark := ""
		if around > 0 {
			mark = "  "
			if i >= lo && i <= hi {
				mark = "> "
			}
		}
		r := &m.rows[i]
		switch r.kind {
		case rowLine:
			sb.WriteString(mark + askLine(r.line) + "\n")
		case rowSplit:
			if r.left != nil {
				sb.WriteString(mark + askLine(r.left) + "\n")
			}
			if r.right != nil && r.right != r.left && r.right.Kind != diff.Context {
				sb.WriteString(mark + askLine(r.right) + "\n")
			}
		case rowThread:
			if i >= lo && i <= hi {
				threads = append(threads, r.thread)
			}
		}
	}
	for _, t := range threads {
		sb.WriteString("\nReview thread on these lines:\n")
		for _, c := range t.Comments {
			sb.WriteString("@" + c.Author + ": " + strings.TrimSpace(c.Body) + "\n")
		}
	}
	return sb.String()
}

// askLine formats one diff line for the prompt: "  12 + text".
func askLine(l *diff.Line) string {
	mark, n := " ", l.NewNum
	switch l.Kind {
	case diff.Add:
		mark = "+"
	case diff.Del:
		mark, n = "-", l.OldNum
	}
	return fmt.Sprintf("%4d %s %s", n, mark, l.Text)
}
