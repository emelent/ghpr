// Package ui implements the bubbletea TUI for reviewing pull requests.
package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ghpr/internal/diff"
	"ghpr/internal/gh"
	"ghpr/internal/state"
)

type screen int

const (
	screenPicker screen = iota
	screenDiff
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayInput
	overlayReview
	overlayMerge
	overlayState // close / reopen confirmation
	overlayHelp
)

type inputKind int

const (
	inputComment inputKind = iota
	inputReply
	inputReview
	inputPRComment
	inputMerge
)

const (
	filesPanelWidth = 34
	inputPanelH     = 10
	statusTimeout   = 5 * time.Second
)

// Model is the root bubbletea model.
type Model struct {
	client *gh.Client
	number int
	hl     *Highlighter

	width, height int
	screen        screen
	overlay       overlayKind

	// picker
	list      list.Model
	listState string // open, closed, merged or all

	fromPicker bool // the PR was opened from the list; q returns to it

	// merge menu
	mergeMethod gh.MergeMethod // chosen method awaiting confirmation ("" = none)
	mergeDelete bool           // delete the head branch after merging

	// data
	pr      *gh.PR
	files   []diff.File
	threads []gh.Thread
	pending int // outstanding loads

	// viewed-file tracking (persisted via store; nil store disables it)
	store        *state.Store
	fingerprints []string // per file, computed at parse time

	// diff view state
	fileIdx      int
	fileScroll   int
	filesFocused bool
	showFiles    bool
	// file panel: tree or flat list
	tree      bool
	collapsed map[string]bool
	treeNodes []treeNode
	treeSel   string // path of the selected tree node (dir or file)
	split     bool
	rows      []row
	rowStart  []int
	totalH    int
	threadH   map[int]int
	numW      int
	cursor    int
	scroll    int
	selecting bool // visual line selection active
	selAnchor int  // row index where the selection started
	spans     map[*diff.Line][]Span
	spanCache map[int]map[*diff.Line][]Span

	// full-file view
	full        bool               // show whole files instead of hunks
	fullFiles   map[int]*diff.File // expanded files by index
	fullSpans   map[int]map[*diff.Line][]Span
	fullPending map[int]bool // fetches in flight
	fullFailed  map[int]bool // fetch failed; fall back to hunks

	// async / status
	spinner   spinner.Model
	busy      string
	status    string
	statusErr bool
	statusSeq int

	// text entry
	ta         textarea.Model
	inputTitle string
	inKind     inputKind
	inComment  gh.LineComment
	inThread   *gh.Thread
	inEvent    gh.ReviewEvent

	fatal error
}

// New creates a model. If number is 0 a PR picker is shown first. syntax is
// the chroma style used for highlighting; call ApplyTheme beforehand to pick
// the UI palette.
func New(client *gh.Client, number int, syntax string) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colWarn).Background(colBarBg)

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.SetHeight(inputPanelH - 3)
	ta.SetVirtualCursor(true)
	tas := ta.Styles()
	tas.Focused.CursorLine = lipgloss.NewStyle()
	tas.Focused.Prompt = lipgloss.NewStyle().Foreground(colWarn)
	tas.Blurred.Prompt = lipgloss.NewStyle().Foreground(colDim)
	ta.SetStyles(tas)

	m := &Model{
		client:      client,
		number:      number,
		hl:          NewHighlighter(syntax),
		spinner:     sp,
		ta:          ta,
		showFiles:   true,
		tree:        true,
		collapsed:   map[string]bool{},
		threadH:     map[int]int{},
		spanCache:   map[int]map[*diff.Line][]Span{},
		fullFiles:   map[int]*diff.File{},
		fullSpans:   map[int]map[*diff.Line][]Span{},
		fullPending: map[int]bool{},
		fullFailed:  map[int]bool{},
	}
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(colAccent).BorderForeground(colAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(colDim).BorderForeground(colAccent)
	l := list.New(nil, d, 0, 0)
	m.listState = "open"
	l.Title = m.listTitle()
	l.Styles.Title = lipgloss.NewStyle().Background(colSelBg).Foreground(colText).Padding(0, 1)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	m.list = l
	m.screen = screenDiff
	if number == 0 {
		m.screen = screenPicker
	}
	return m
}

// ---------- messages ----------

type prListMsg struct {
	prs []gh.PRSummary
	err error
}
type prMsg struct {
	pr  *gh.PR
	err error
}
type diffMsg struct {
	files []diff.File
	err   error
}
type threadsMsg struct {
	threads []gh.Thread
	err     error
}
type fileContentMsg struct {
	idx  int
	path string
	text string
	err  error
}
type actionMsg struct {
	label   string
	err     error
	refresh bool // reload threads (and PR) afterwards
}
type clearStatusMsg struct{ seq int }

type prItem struct{ s gh.PRSummary }

func (p prItem) Title() string {
	t := fmt.Sprintf("#%d  %s", p.s.Number, p.s.Title)
	if p.s.IsDraft {
		t += "  [draft]"
	}
	return t
}
func (p prItem) Description() string {
	d := p.s.ReviewDecision
	if d == "" {
		d = "no review"
	}
	prefix := ""
	if p.s.State != "" && p.s.State != "OPEN" {
		prefix = p.s.State + " · "
	}
	return fmt.Sprintf("%s@%s · %s · %s · updated %s", prefix, p.s.Author.Login, p.s.HeadRefName, d, ago(p.s.UpdatedAt))
}
func (p prItem) FilterValue() string {
	return p.Title() + " " + p.s.Author.Login + " " + p.s.HeadRefName
}

// ---------- commands ----------

func (m *Model) fetchList() tea.Cmd {
	c, st := m.client, m.listState
	return func() tea.Msg {
		prs, err := c.ListPRs(st, 100)
		return prListMsg{prs, err}
	}
}

var listStates = []string{"open", "closed", "merged", "all"}

// SetListState sets the PR picker filter (open, closed, merged, all).
func (m *Model) SetListState(s string) {
	for _, v := range listStates {
		if v == s {
			m.listState = s
			m.list.Title = m.listTitle()
			return
		}
	}
}

func (m *Model) listTitle() string {
	return fmt.Sprintf("%s pull requests · %s", strings.ToUpper(m.listState[:1])+m.listState[1:], m.client.Repo)
}

// cycleListState moves to the next state filter and reloads the list.
func (m *Model) cycleListState() tea.Cmd {
	for i, v := range listStates {
		if v == m.listState {
			m.listState = listStates[(i+1)%len(listStates)]
			break
		}
	}
	m.list.Title = m.listTitle()
	m.list.ResetFilter()
	return tea.Batch(m.list.SetItems(nil), m.setBusy("Loading "+m.listState+" pull requests…"), m.fetchList())
}

func (m *Model) fetchPR() tea.Cmd {
	c, n := m.client, m.number
	return func() tea.Msg {
		pr, err := c.ViewPR(n)
		return prMsg{pr, err}
	}
}

func (m *Model) fetchDiff() tea.Cmd {
	c, n := m.client, m.number
	return func() tea.Msg {
		text, err := c.Diff(n)
		if err != nil {
			return diffMsg{nil, err}
		}
		return diffMsg{diff.Parse(text), nil}
	}
}

func (m *Model) fetchThreads() tea.Cmd {
	c, n := m.client, m.number
	return func() tea.Msg {
		th, err := c.ReviewThreads(n)
		return threadsMsg{th, err}
	}
}

// fullEligible reports whether a file has content beyond its hunks worth
// fetching. Added and deleted files are already shown in full by the diff.
func fullEligible(f *diff.File) bool {
	return !f.IsBinary && f.Status != diff.Deleted && f.Status != diff.Added && len(f.Hunks) > 0
}

// ensureFull starts fetching the current file's full content when the full
// view is on and it is not loaded yet.
func (m *Model) ensureFull() tea.Cmd {
	if !m.full || m.pr == nil || len(m.files) == 0 {
		return nil
	}
	idx := m.fileIdx
	f := &m.files[idx]
	if !fullEligible(f) || m.fullFiles[idx] != nil || m.fullPending[idx] || m.fullFailed[idx] {
		return nil
	}
	m.fullPending[idx] = true
	c, ref, path := m.client, m.pr.HeadRefOid, f.Path()
	return tea.Batch(m.setBusy("Loading full file…"), func() tea.Msg {
		text, err := c.FileContent(ref, path)
		return fileContentMsg{idx: idx, path: path, text: text, err: err}
	})
}

func (m *Model) setBusy(msg string) tea.Cmd {
	m.busy = msg
	return m.spinner.Tick
}

func (m *Model) setStatus(msg string, isErr bool) tea.Cmd {
	m.status = msg
	m.statusErr = isErr
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg { return clearStatusMsg{seq} })
}

func (m *Model) loadAll() tea.Cmd {
	m.pending = 3
	return tea.Batch(m.setBusy(fmt.Sprintf("Loading PR #%d…", m.number)), m.fetchPR(), m.fetchDiff(), m.fetchThreads())
}

func (m *Model) reloadThreads(withPR bool) tea.Cmd {
	cmds := []tea.Cmd{m.setBusy("Refreshing…"), m.fetchThreads()}
	m.pending = 1
	if withPR {
		m.pending++
		cmds = append(cmds, m.fetchPR())
	}
	return tea.Batch(cmds...)
}

func (m *Model) action(label string, refresh bool, fn func() error) tea.Cmd {
	busy := m.setBusy(label + "…")
	return tea.Batch(busy, func() tea.Msg {
		return actionMsg{label: label, err: fn(), refresh: refresh}
	})
}

// ---------- init / update ----------

// Init starts loading.
func (m *Model) Init() tea.Cmd {
	if m.screen == screenPicker {
		return tea.Batch(m.setBusy("Loading pull requests…"), m.fetchList())
	}
	return m.loadAll()
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-1)
		m.ta.SetWidth(max(10, m.diffWidth()-2))
		m.invalidateLayout()
		return m, nil

	case spinner.TickMsg:
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case prListMsg:
		m.busy = ""
		if msg.err != nil {
			if len(m.list.Items()) == 0 && m.screen == screenPicker && m.listState == "open" {
				m.fatal = msg.err
				return m, tea.Quit
			}
			return m, m.setStatus("List PRs: "+msg.err.Error(), true)
		}
		items := make([]list.Item, len(msg.prs))
		for i, p := range msg.prs {
			items[i] = prItem{p}
		}
		return m, m.list.SetItems(items)

	case prMsg:
		m.loadDone()
		if msg.err != nil {
			if m.pr == nil {
				m.fatal = msg.err
				return m, tea.Quit
			}
			return m, m.setStatus(msg.err.Error(), true)
		}
		m.pr = msg.pr
		return m, nil

	case diffMsg:
		m.loadDone()
		if msg.err != nil {
			m.fatal = msg.err
			return m, tea.Quit
		}
		m.files = msg.files
		m.fingerprints = make([]string, len(m.files))
		for i := range m.files {
			m.fingerprints[i] = m.files[i].Fingerprint()
		}
		dropped := m.reconcileViewed()
		m.spanCache = map[int]map[*diff.Line][]Span{}
		m.fullFiles = map[int]*diff.File{}
		m.fullSpans = map[int]map[*diff.Line][]Span{}
		m.fullPending = map[int]bool{}
		m.fullFailed = map[int]bool{}
		m.fileIdx = 0
		m.cursor, m.scroll = 0, 0
		m.rebuildRows()
		m.revealFile(0)
		cmds := []tea.Cmd{m.ensureFull()}
		if dropped > 0 {
			cmds = append(cmds, m.setStatus(fmt.Sprintf("%d file(s) changed since you viewed them – unmarked", dropped), false))
		}
		return m, tea.Batch(cmds...)

	case fileContentMsg:
		delete(m.fullPending, msg.idx)
		if len(m.fullPending) == 0 && m.pending == 0 {
			m.busy = ""
		}
		if msg.idx >= len(m.files) || m.files[msg.idx].Path() != msg.path {
			return m, nil // stale: diff was reloaded meanwhile
		}
		if msg.err != nil {
			m.fullFailed[msg.idx] = true
			return m, m.setStatus("Full file unavailable: "+msg.err.Error(), true)
		}
		m.fullFiles[msg.idx] = diff.Expand(&m.files[msg.idx], msg.text)
		if msg.idx == m.fileIdx {
			m.rebuildRowsKeepLine()
		}
		return m, nil

	case threadsMsg:
		m.loadDone()
		if msg.err != nil {
			return m, m.setStatus("threads: "+msg.err.Error(), true)
		}
		m.threads = msg.threads
		m.rebuildRows()
		return m, nil

	case actionMsg:
		m.busy = ""
		if msg.err != nil {
			return m, m.setStatus(msg.label+" failed: "+msg.err.Error(), true)
		}
		cmds := []tea.Cmd{m.setStatus(msg.label+" ✓", false)}
		if msg.refresh {
			cmds = append(cmds, m.reloadThreads(true))
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	if m.screen == screenPicker {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) loadDone() {
	if m.pending > 0 {
		m.pending--
	}
	if m.pending == 0 {
		m.busy = ""
	}
}

// Fatal returns the error that caused the program to exit, if any.
func (m *Model) Fatal() error { return m.fatal }

// ---------- keys ----------

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.screen == screenPicker {
		if m.list.FilterState() != list.Filtering {
			switch key {
			case "q":
				return m, tea.Quit
			case "s":
				return m, m.cycleListState()
			case "enter", "l", "right":
				if it, ok := m.list.SelectedItem().(prItem); ok {
					m.number = it.s.Number
					m.screen = screenDiff
					m.fromPicker = true
					return m, m.loadAll()
				}
			}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch m.overlay {
	case overlayInput:
		switch key {
		case "esc":
			m.closeInput()
			return m, nil
		case "super+s", "meta+s", "ctrl+s":
			// cmd+s on macOS (reported as super/meta by terminals that
			// support the kitty keyboard protocol); ctrl+s elsewhere.
			return m, m.submitInput()
		}
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		return m, cmd

	case overlayReview:
		switch key {
		case "a":
			m.overlay = overlayNone
			m.openInput(inputReview, fmt.Sprintf("Approve PR #%d — optional comment", m.number))
			m.inEvent = gh.Approve
		case "r":
			m.overlay = overlayNone
			m.openInput(inputReview, fmt.Sprintf("Request changes on PR #%d — explain what needs to change", m.number))
			m.inEvent = gh.RequestChanges
		case "c":
			m.overlay = overlayNone
			m.openInput(inputReview, fmt.Sprintf("Review comment on PR #%d", m.number))
			m.inEvent = gh.CommentReview
		case "esc", "v", "q":
			m.overlay = overlayNone
		}
		return m, nil

	case overlayMerge:
		return m.handleMergeKey(key)

	case overlayState:
		switch key {
		case "d":
			m.mergeDelete = !m.mergeDelete
		case "y", "enter":
			m.overlay = overlayNone
			n, del := m.number, m.mergeDelete
			if m.pr.State == "OPEN" {
				return m, m.action("Close PR", true, func() error { return m.client.Close(n, del) })
			}
			return m, m.action("Reopen PR", true, func() error { return m.client.Reopen(n) })
		case "esc", "n", "q", "X":
			m.overlay = overlayNone
		}
		return m, nil

	case overlayHelp:
		m.overlay = overlayNone
		return m, nil
	}

	if m.filesFocused {
		if handled, cmd := m.handleFilesKey(key); handled {
			return m, cmd
		}
	}

	switch key {
	case "q":
		if m.selecting {
			m.clearSelection()
			return m, nil
		}
		if m.fromPicker {
			return m, m.backToList()
		}
		return m, tea.Quit
	case "Q":
		return m, tea.Quit
	case "b", "backspace":
		return m, m.backToList()
	case "M":
		if m.busy == "" && m.pr != nil {
			if m.pr.State != "OPEN" {
				return m, m.setStatus("PR is "+strings.ToLower(m.pr.State)+"; only open PRs can be merged", true)
			}
			m.overlay = overlayMerge
			m.mergeMethod = ""
		}
	case "X":
		if m.busy == "" && m.pr != nil {
			if m.pr.State == "MERGED" {
				return m, m.setStatus("Merged PRs cannot be reopened", true)
			}
			m.overlay = overlayState
		}
	case "t":
		m.tree = !m.tree
		m.fileScroll = 0
		m.revealFile(m.fileIdx)
	case "esc":
		m.clearSelection()
	case "V":
		m.toggleSelection()
	case "?":
		m.overlay = overlayHelp
	case "tab":
		m.filesFocused = !m.filesFocused
		if m.filesFocused {
			m.showFiles = true
		}
	case "f":
		m.showFiles = !m.showFiles
		if !m.showFiles {
			m.filesFocused = false
		}
		m.invalidateLayout()
	case "s":
		m.split = !m.split
		m.clearSelection()
		m.rebuildRowsKeepLine()
	case "F":
		m.full = !m.full
		m.clearSelection()
		m.rebuildRowsKeepLine()
		return m, m.ensureFull()
	case "m":
		return m, m.toggleViewed()
	case "J":
		m.jumpChange(1)
	case "K":
		m.jumpChange(-1)
	case "R":
		return m, m.loadAll()
	case "o":
		return m, m.action("Open in browser", false, func() error { return m.client.OpenInBrowser(m.number) })
	case "v":
		if m.busy == "" {
			m.overlay = overlayReview
		}
	case "C":
		m.openInput(inputPRComment, fmt.Sprintf("Comment on PR #%d", m.number))
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "ctrl+d", "pgdown", "space":
		m.moveCursor(m.viewportH() / 2)
	case "ctrl+u", "pgup":
		m.moveCursor(-m.viewportH() / 2)
	case "ctrl+f":
		m.moveCursor(m.viewportH())
	case "ctrl+b":
		m.moveCursor(-m.viewportH())
	case "g", "home":
		m.cursor = 0
		m.scroll = 0
	case "G", "end":
		m.cursor = max(0, len(m.rows)-1)
	case "]", "l", "right":
		return m, m.selectFile(m.fileIdx + 1)
	case "[", "h", "left":
		return m, m.selectFile(m.fileIdx - 1)
	case "n":
		return m, m.jumpThread(1)
	case "N":
		return m, m.jumpThread(-1)
	case "c":
		return m, m.startComment()
	case "r":
		return m, m.startReply()
	case "x":
		return m, m.toggleResolve()
	}
	return m, nil
}

// ---------- navigation ----------

func (m *Model) selectFile(i int) tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	if i < 0 {
		i = 0
	}
	if i >= len(m.files) {
		i = len(m.files) - 1
	}
	if i == m.fileIdx {
		return nil
	}
	m.fileIdx = i
	m.cursor, m.scroll = 0, 0
	m.clearSelection()
	m.rebuildRows()
	m.revealFile(i)
	return m.ensureFull()
}

// ---------- file panel ----------

// rebuildTree recomputes the visible tree nodes.
func (m *Model) rebuildTree() {
	m.treeNodes = buildTree(m.files, m.collapsed, m.isViewed)
}

// revealFile expands the ancestors of file i and selects it in the panel.
func (m *Model) revealFile(i int) {
	if i < 0 || i >= len(m.files) {
		return
	}
	path := m.files[i].Path()
	for _, d := range dirsOf(path) {
		delete(m.collapsed, d)
	}
	m.treeSel = path
	m.rebuildTree()
}

// treeIndex returns the index of the selected node in treeNodes (or the
// current file's node when the selection is stale).
func (m *Model) treeIndex() int {
	for i := range m.treeNodes {
		if m.treeNodes[i].path == m.treeSel {
			return i
		}
	}
	if m.fileIdx < len(m.files) {
		p := m.files[m.fileIdx].Path()
		for i := range m.treeNodes {
			if !m.treeNodes[i].isDir && m.treeNodes[i].path == p {
				return i
			}
		}
	}
	return 0
}

// handleFilesKey processes keys while the file panel is focused. It returns
// handled=false for keys that should fall through to the normal bindings.
func (m *Model) handleFilesKey(key string) (bool, tea.Cmd) {
	if len(m.files) == 0 {
		return false, nil
	}
	if !m.tree {
		switch key {
		case "j", "down":
			return true, m.selectFile(m.fileIdx + 1)
		case "k", "up":
			return true, m.selectFile(m.fileIdx - 1)
		case "g", "home":
			return true, m.selectFile(0)
		case "G", "end":
			return true, m.selectFile(len(m.files) - 1)
		case "enter", "space":
			m.filesFocused = false
			return true, nil
		}
		return false, nil
	}
	if len(m.treeNodes) == 0 {
		m.rebuildTree()
	}
	idx := m.treeIndex()
	node := &m.treeNodes[idx]
	moveTo := func(n int) tea.Cmd {
		if n < 0 || n >= len(m.treeNodes) {
			return nil
		}
		m.treeSel = m.treeNodes[n].path
		if !m.treeNodes[n].isDir {
			return m.selectFile(m.treeNodes[n].fileIdx)
		}
		return nil
	}
	move := func(d int) tea.Cmd { return moveTo(idx + d) }
	switch key {
	case "j", "down":
		return true, move(1)
	case "k", "up":
		return true, move(-1)
	case "g", "home":
		return true, moveTo(0)
	case "G", "end":
		return true, moveTo(len(m.treeNodes) - 1)
	case "enter", "space":
		if node.isDir {
			m.collapsed[node.path] = !m.collapsed[node.path]
			m.rebuildTree()
			return true, nil
		}
		m.filesFocused = false
		return true, m.selectFile(node.fileIdx)
	case "l", "right":
		if node.isDir && m.collapsed[node.path] {
			delete(m.collapsed, node.path)
			m.rebuildTree()
		}
		return true, nil
	case "h", "left":
		if node.isDir && !m.collapsed[node.path] {
			m.collapsed[node.path] = true
			m.rebuildTree()
			return true, nil
		}
		// Jump to the parent directory node.
		parent := node.path[:max(0, strings.LastIndex(node.path, "/"))]
		for i := idx - 1; i >= 0; i-- {
			n := &m.treeNodes[i]
			if n.isDir && n.depth == node.depth-1 && strings.HasPrefix(parent, n.path) {
				m.treeSel = n.path
				break
			}
		}
		return true, nil
	case "H":
		for i := range m.treeNodes {
			if m.treeNodes[i].isDir {
				m.collapsed[m.treeNodes[i].path] = true
			}
		}
		m.rebuildTree()
		m.revealFile(m.fileIdx)
		return true, nil
	case "L":
		m.collapsed = map[string]bool{}
		m.rebuildTree()
		return true, nil
	}
	return false, nil
}

// rowChanged reports whether row i shows an added or removed line.
func (m *Model) rowChanged(i int) bool {
	if i < 0 || i >= len(m.rows) {
		return false
	}
	r := &m.rows[i]
	switch r.kind {
	case rowLine:
		return r.line.Kind != diff.Context
	case rowSplit:
		return (r.left != nil && r.left.Kind != diff.Context) || (r.right != nil && r.right.Kind != diff.Context)
	}
	return false
}

// jumpChange moves the cursor to the start of the next (dir > 0) or previous
// (dir < 0) block of contiguous changed lines in the current file.
func (m *Model) jumpChange(dir int) {
	n := len(m.rows)
	if n == 0 {
		return
	}
	if dir > 0 {
		i := m.cursor
		for i < n && m.rowChanged(i) {
			i++ // leave the block we are in
		}
		for i < n && !m.rowChanged(i) {
			i++
		}
		if i < n {
			m.cursor = i
		}
		return
	}
	i := m.cursor
	if m.rowChanged(i) && m.rowChanged(i-1) {
		for i > 0 && m.rowChanged(i-1) {
			i-- // inside a block: go to its start
		}
		m.cursor = i
		return
	}
	i--
	for i >= 0 && !m.rowChanged(i) {
		i--
	}
	if i < 0 {
		return
	}
	for i > 0 && m.rowChanged(i-1) {
		i--
	}
	m.cursor = i
}

// ---------- selection ----------

func (m *Model) toggleSelection() {
	if m.selecting {
		m.clearSelection()
		return
	}
	if len(m.rows) == 0 {
		return
	}
	m.selecting = true
	m.selAnchor = m.cursor
}

func (m *Model) clearSelection() {
	m.selecting = false
	m.selAnchor = 0
}

// selection returns the inclusive row range currently selected.
func (m *Model) selection() (lo, hi int, ok bool) {
	if !m.selecting {
		return 0, 0, false
	}
	lo, hi = m.selAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

// inSelection reports whether row i is part of the active selection.
func (m *Model) inSelection(i int) bool {
	lo, hi, ok := m.selection()
	return ok && i >= lo && i <= hi
}

// selectedLineCount counts diff lines inside the selection.
func (m *Model) selectedLineCount() int {
	lo, hi, ok := m.selection()
	if !ok {
		return 0
	}
	n := 0
	for i := lo; i <= hi && i < len(m.rows); i++ {
		if _, _, isLine := m.rows[i].nums(); isLine {
			n++
		}
	}
	return n
}

func (m *Model) moveCursor(d int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

// jumpThread moves the cursor to the next/previous thread row, crossing
// file boundaries and wrapping around.
func (m *Model) jumpThread(dir int) tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	// Current file first.
	for i := m.cursor + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].kind == rowThread {
			m.cursor = i
			return nil
		}
	}
	hasThread := func(path string) bool {
		for _, t := range m.threads {
			if t.Path == path {
				return true
			}
		}
		return false
	}
	for step := 1; step <= len(m.files); step++ {
		fi := ((m.fileIdx+dir*step)%len(m.files) + len(m.files)) % len(m.files)
		if !hasThread(m.files[fi].Path()) {
			continue
		}
		m.fileIdx = fi
		m.scroll = 0
		m.clearSelection()
		m.rebuildRows()
		m.cursor = 0
		if dir < 0 {
			m.cursor = len(m.rows) - 1
		}
		for i := m.cursor; i >= 0 && i < len(m.rows); i += dir {
			if m.rows[i].kind == rowThread {
				m.cursor = i
				break
			}
		}
		return m.ensureFull()
	}
	return nil
}

// ---------- layout ----------

func (m *Model) filesWidth() int {
	if !m.showFiles {
		return 0
	}
	w := filesPanelWidth
	if m.width < 100 {
		w = max(20, m.width/4)
	}
	return w
}

func (m *Model) diffWidth() int {
	w := m.width
	if m.showFiles {
		w -= m.filesWidth() + 1
	}
	return max(10, w)
}

func (m *Model) mainHeight() int { return max(3, m.height-3) }

// viewportH is the number of diff rows visible (excluding pane header).
func (m *Model) viewportH() int {
	h := m.mainHeight() - 2
	if m.overlay == overlayInput {
		h -= inputPanelH
	}
	return max(1, h)
}

func (m *Model) invalidateLayout() {
	m.threadH = map[int]int{}
	m.computeOffsets()
}

func (m *Model) rebuildRows() {
	if len(m.files) == 0 {
		m.rows = nil
		m.rowStart = nil
		return
	}
	f := m.viewFile()
	cache := m.spanCache
	if f.Full {
		cache = m.fullSpans
	}
	if sp, ok := cache[m.fileIdx]; ok {
		m.spans = sp
	} else {
		m.spans = m.hl.HighlightFile(f)
		cache[m.fileIdx] = m.spans
	}
	m.rows = buildRows(f, m.split, m.threads)
	maxNum := 1
	for _, h := range f.Hunks {
		maxNum = max(maxNum, h.OldStart+h.OldCount)
		maxNum = max(maxNum, h.NewStart+h.NewCount)
	}
	m.numW = max(3, digits(maxNum))
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	m.invalidateLayout()
}

// viewFile returns the file to render: the expanded version when the full
// view is on and loaded, otherwise the diff hunks.
func (m *Model) viewFile() *diff.File {
	if m.full {
		if ff := m.fullFiles[m.fileIdx]; ff != nil {
			return ff
		}
	}
	return &m.files[m.fileIdx]
}

// showingFull reports whether the current file is rendered in full.
func (m *Model) showingFull() bool {
	return len(m.files) > 0 && m.viewFile().Full
}

// rebuildRowsKeepLine rebuilds rows and moves the cursor to the row showing
// the same line (or thread) it was on before, so toggling layouts does not
// lose the reader's place.
func (m *Model) rebuildRowsKeepLine() {
	var threadID string
	oldNum, newNum := 0, 0
	if r := m.currentRow(); r != nil {
		if r.kind == rowThread {
			threadID = r.thread.ID
		} else {
			oldNum, newNum, _ = r.nums()
		}
	}
	m.rebuildRows()
	for i := range m.rows {
		r := &m.rows[i]
		if threadID != "" {
			if r.kind == rowThread && r.thread.ID == threadID {
				m.cursor = i
				return
			}
			continue
		}
		o, n, ok := r.nums()
		if ok && (newNum > 0 && n == newNum || newNum == 0 && oldNum > 0 && o == oldNum) {
			m.cursor = i
			return
		}
	}
}

func (m *Model) computeOffsets() {
	m.rowStart = make([]int, len(m.rows))
	y := 0
	for i := range m.rows {
		m.rowStart[i] = y
		y += m.rowHeight(i)
	}
	m.totalH = y
}

func (m *Model) ensureCursorVisible(avail int) {
	if len(m.rows) == 0 {
		m.scroll = 0
		return
	}
	top := m.rowStart[m.cursor]
	bottom := top + m.rowHeight(m.cursor)
	if top < m.scroll {
		m.scroll = top
	}
	if bottom > m.scroll+avail {
		m.scroll = bottom - avail
	}
	if m.scroll > max(0, m.totalH-avail) {
		m.scroll = max(0, m.totalH-avail)
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// ---------- text entry & actions ----------

func (m *Model) openInput(kind inputKind, title string) {
	m.inKind = kind
	m.inputTitle = title
	m.ta.Reset()
	m.ta.SetWidth(max(10, m.diffWidth()-2))
	m.ta.Focus()
	m.overlay = overlayInput
}

func (m *Model) closeInput() {
	m.ta.Blur()
	m.overlay = overlayNone
	m.inThread = nil
	m.clearSelection()
}

func (m *Model) currentRow() *row {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) startComment() tea.Cmd {
	if m.busy != "" || m.pr == nil {
		return nil
	}
	f := &m.files[m.fileIdx]
	if lo, hi, ok := m.selection(); ok {
		start, end, startSide, side, ok := rangeAnchor(m.rows, lo, hi)
		if !ok {
			return m.setStatus("Selection contains no diff lines", true)
		}
		if !f.InDiff(startSide, start) || !f.InDiff(side, end) {
			return m.setStatus("GitHub only accepts comments on lines that are part of the diff", true)
		}
		m.inComment = gh.LineComment{Path: f.Path(), Line: end, Side: side, StartLine: start, StartSide: startSide}
		if !m.inComment.IsRange() {
			m.inComment.StartLine, m.inComment.StartSide = 0, ""
			m.openInput(inputComment, fmt.Sprintf("New comment on %s:%d (%s)", f.Path(), end, side))
			return nil
		}
		loc := fmt.Sprintf("%s:%d-%d (%s)", f.Path(), start, end, side)
		if startSide != side {
			loc = fmt.Sprintf("%s:%s %d → %s %d", f.Path(), startSide, start, side, end)
		}
		m.openInput(inputComment, "New comment on "+loc)
		return nil
	}
	r := m.currentRow()
	if r == nil {
		return nil
	}
	line, side, ok := r.anchor()
	if !ok {
		return m.setStatus("Move the cursor onto a diff line to comment (V to select a range)", true)
	}
	if !f.InDiff(side, line) {
		return m.setStatus("GitHub only accepts comments on lines that are part of the diff", true)
	}
	m.inComment = gh.LineComment{Path: f.Path(), Line: line, Side: side}
	m.openInput(inputComment, fmt.Sprintf("New comment on %s:%d (%s)", f.Path(), line, side))
	return nil
}

func (m *Model) startReply() tea.Cmd {
	if m.busy != "" {
		return nil
	}
	r := m.currentRow()
	if r == nil || r.kind != rowThread {
		return m.setStatus("Move the cursor onto a thread to reply", true)
	}
	if len(r.thread.Comments) == 0 {
		return m.setStatus("Thread has no comments to reply to", true)
	}
	m.inThread = r.thread
	first := r.thread.Comments[0]
	m.openInput(inputReply, fmt.Sprintf("Reply to @%s on %s:%d", first.Author, r.thread.Path, max(r.thread.Line, r.thread.OriginalLine)))
	return nil
}

func (m *Model) toggleResolve() tea.Cmd {
	if m.busy != "" {
		return nil
	}
	r := m.currentRow()
	if r == nil || r.kind != rowThread {
		return m.setStatus("Move the cursor onto a thread to resolve it", true)
	}
	t := r.thread
	if t.IsResolved {
		return m.action("Unresolve thread", true, func() error { return m.client.UnresolveThread(t.ID) })
	}
	return m.action("Resolve thread", true, func() error { return m.client.ResolveThread(t.ID) })
}

// backToList leaves the diff view for the PR picker and refreshes it.
func (m *Model) backToList() tea.Cmd {
	m.screen = screenPicker
	m.overlay = overlayNone
	m.pr, m.files, m.threads, m.rows, m.rowStart = nil, nil, nil, nil, nil
	m.fingerprints = nil
	m.pending = 0
	m.busy = ""
	m.status = ""
	m.clearSelection()
	m.closeInput()
	m.fromPicker = false
	m.list.Title = m.listTitle()
	return tea.Batch(m.setBusy("Loading pull requests…"), m.fetchList())
}

// handleMergeKey drives the merge menu: pick a method, toggle branch
// deletion, then confirm with y.
func (m *Model) handleMergeKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n", "q", "M":
		m.overlay = overlayNone
		m.mergeMethod = ""
	case "d":
		m.mergeDelete = !m.mergeDelete
	case "m", "s":
		method := gh.MergeCommit
		if key == "s" {
			method = gh.Squash
		}
		m.overlay = overlayNone
		m.mergeMethod = method
		del := "off"
		if m.mergeDelete {
			del = "on"
		}
		m.openInput(inputMerge, fmt.Sprintf("Merge PR #%d via %s (delete branch: %s) — edit the commit message; first line is the subject", m.number, method, del))
		subject, body := gh.DefaultMergeMessage(m.pr, method)
		if body != "" {
			m.ta.SetValue(subject + "\n\n" + body)
		} else {
			m.ta.SetValue(subject)
		}
		m.ta.MoveToBegin()
	case "r":
		m.mergeMethod = gh.Rebase
	case "y", "enter":
		if m.mergeMethod != gh.Rebase {
			return m, nil
		}
		return m, m.doMerge(gh.MergeOptions{Method: gh.Rebase, DeleteBranch: m.mergeDelete})
	}
	return m, nil
}

// doMerge runs the merge and refreshes the PR afterwards.
func (m *Model) doMerge(o gh.MergeOptions) tea.Cmd {
	m.overlay = overlayNone
	m.mergeMethod = ""
	n := m.number
	return m.action(fmt.Sprintf("Merge (%s)", o.Method), true, func() error {
		return m.client.Merge(n, o)
	})
}

// splitMessage separates a commit message into subject (first line) and
// body (the rest, trimmed).
func splitMessage(text string) (subject, body string) {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r", "")
	subject, body, _ = strings.Cut(text, "\n")
	return strings.TrimSpace(subject), strings.TrimSpace(body)
}

func (m *Model) submitInput() tea.Cmd {
	body := strings.TrimSpace(m.ta.Value())
	kind := m.inKind
	needsBody := kind != inputMerge && (kind != inputReview || m.inEvent != gh.Approve)
	if needsBody && body == "" {
		return m.setStatus("Comment body is empty", true)
	}
	thread := m.inThread // closeInput clears it
	m.closeInput()
	c, n := m.client, m.number
	switch kind {
	case inputComment:
		lc, sha := m.inComment, m.pr.HeadRefOid
		lc.Body = body
		return m.action("Post comment", true, func() error {
			return c.AddLineComment(n, sha, lc)
		})
	case inputReply:
		if thread == nil || len(thread.Comments) == 0 {
			return m.setStatus("No thread selected for reply", true)
		}
		id := thread.Comments[0].DatabaseID
		return m.action("Post reply", true, func() error { return c.ReplyToComment(n, id, body) })
	case inputPRComment:
		return m.action("Post PR comment", false, func() error { return c.Comment(n, body) })
	case inputMerge:
		subject, msgBody := splitMessage(body)
		return m.doMerge(gh.MergeOptions{Method: m.mergeMethod, DeleteBranch: m.mergeDelete, Subject: subject, Body: msgBody})
	case inputReview:
		ev := m.inEvent
		label := map[gh.ReviewEvent]string{gh.Approve: "Approve", gh.RequestChanges: "Request changes", gh.CommentReview: "Submit review"}[ev]
		return m.action(label, true, func() error { return c.Review(n, ev, body) })
	}
	return nil
}

// ---------- view ----------

// View renders the UI into a full-screen bubbletea view.
func (m *Model) View() tea.View {
	v := tea.NewView(m.view())
	v.AltScreen = true
	v.BackgroundColor = color.Color(colAppBg)
	return v
}

// view renders the UI as a string.
func (m *Model) view() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.screen == screenPicker {
		return m.list.View() + "\n" + m.renderStatus(m.width)
	}

	header := m.renderHeader(m.width)
	mainH := m.mainHeight()
	dw := m.diffWidth()

	var right []string
	if m.overlay == overlayHelp {
		right = m.renderHelp(dw, mainH)
	} else {
		diffH := mainH
		if m.overlay == overlayInput {
			diffH -= inputPanelH
		}
		right = m.renderDiff(dw, diffH)
		if m.overlay == overlayInput {
			right = append(right, m.renderInput(dw, inputPanelH)...)
		}
	}

	var body []string
	if m.showFiles {
		left := m.renderFiles(m.filesWidth(), mainH)
		sep := styBorder.Render("│")
		for i := 0; i < mainH; i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			body = append(body, l+sep+r)
		}
	} else {
		body = right
	}
	for len(body) < mainH {
		body = append(body, "")
	}

	var sb strings.Builder
	sb.WriteString(strings.Join(header, "\n"))
	sb.WriteString("\n")
	sb.WriteString(strings.Join(body[:mainH], "\n"))
	sb.WriteString("\n")
	sb.WriteString(m.renderStatus(m.width))
	return sb.String()
}

// SetSplit sets the initial diff layout.
func (m *Model) SetSplit(v bool) { m.split = v }

// SetStore enables persisted "viewed" marks for files.
func (m *Model) SetStore(s *state.Store) { m.store = s }

// ---------- viewed files ----------

func (m *Model) prKey() string { return state.PRKey(m.client.Repo, m.number) }

// viewedInfo returns the viewed record for a file, if marked.
func (m *Model) viewedInfo(path string) (state.Viewed, bool) {
	if m.store == nil {
		return state.Viewed{}, false
	}
	return m.store.Get(m.prKey(), path)
}

func (m *Model) isViewed(path string) bool {
	_, ok := m.viewedInfo(path)
	return ok
}

// viewedCount returns how many of the PR's files are marked viewed.
func (m *Model) viewedCount() int {
	n := 0
	for i := range m.files {
		if m.isViewed(m.files[i].Path()) {
			n++
		}
	}
	return n
}

// reconcileViewed drops viewed marks for files whose diff changed since they
// were viewed and returns how many were dropped. Files that left the PR
// entirely are dropped too.
func (m *Model) reconcileViewed() int {
	if m.store == nil {
		return 0
	}
	key := m.prKey()
	current := map[string]string{}
	for i := range m.files {
		current[m.files[i].Path()] = m.fingerprints[i]
	}
	dropped := 0
	for _, path := range m.store.Paths(key) {
		v, _ := m.store.Get(key, path)
		fp, present := current[path]
		if !present || fp != v.Fingerprint {
			m.store.Delete(key, path)
			dropped++
		}
	}
	if dropped > 0 {
		_ = m.store.Save()
	}
	return dropped
}

// toggleViewed marks or unmarks the current file and advances to the next
// file after marking.
func (m *Model) toggleViewed() tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	if m.store == nil {
		return m.setStatus("Viewed marks are disabled (state store unavailable)", true)
	}
	f := &m.files[m.fileIdx]
	key, path := m.prKey(), f.Path()
	if m.isViewed(path) {
		m.store.Delete(key, path)
		if err := m.store.Save(); err != nil {
			return m.setStatus("Save viewed state: "+err.Error(), true)
		}
		return m.setStatus("Unmarked "+path, false)
	}
	sha := ""
	if m.pr != nil {
		sha = m.pr.HeadRefOid
	}
	m.store.Set(key, path, state.Viewed{ViewedAt: time.Now(), HeadSHA: sha, Fingerprint: m.fingerprints[m.fileIdx]})
	if err := m.store.Save(); err != nil {
		return m.setStatus("Save viewed state: "+err.Error(), true)
	}
	cmds := []tea.Cmd{m.setStatus(fmt.Sprintf("Viewed %s (%d/%d)", path, m.viewedCount(), len(m.files)), false)}
	if m.fileIdx+1 < len(m.files) {
		cmds = append(cmds, m.selectFile(m.fileIdx+1))
	}
	return tea.Batch(cmds...)
}
