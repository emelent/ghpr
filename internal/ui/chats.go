package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ghpr/internal/keys"
	"ghpr/internal/state"
)

// ---------- Claude conversations screen (P) ----------

// chItemH is the number of lines one conversation takes in the list:
// location, first question, latest answer, spacer.
const chItemH = 4

// openChats (P) lists the PR's conversations with Claude, the active one
// selected.
func (m *Model) openChats() tea.Cmd {
	if len(m.chats) == 0 {
		return m.setStatus("No conversations with Claude on this PR yet ("+m.keys.First(keys.Diff, "ask")+" asks about the line or selection)", true)
	}
	m.screen = screenChats
	m.overlay = overlayNone
	m.chIdx = len(m.chats) - 1
	for i := range m.chats {
		if m.chats[i].ID == m.chatID {
			m.chIdx = i
		}
	}
	return nil
}

// handleChatsKey drives the conversations screen.
func (m *Model) handleChatsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "P", "q":
		m.screen = screenDiff
		m.backTo = screenDiff
	case "j", "down":
		if m.chIdx+1 < len(m.chats) {
			m.chIdx++
		}
	case "k", "up":
		if m.chIdx > 0 {
			m.chIdx--
		}
	case "g", "home":
		m.chIdx = 0
	case "G", "end":
		m.chIdx = max(0, len(m.chats)-1)
	case "l", "right", "enter":
		return m, m.openChat()
	case "d", "x":
		if m.chIdx < len(m.chats) {
			m.removeChat(m.chIdx)
			save := m.saveChats()
			if len(m.chats) == 0 {
				m.screen = screenDiff
				m.backTo = screenDiff
				return m, tea.Batch(save, m.setStatus("Last conversation removed", false))
			}
			return m, save
		}
	case "?":
		m.screen = screenDiff
		m.overlay = overlayHelp
	case "Q", "ctrl+c":
		m.savePosition()
		return m, tea.Quit
	}
	return m, nil
}

// openChat shows the selected conversation in the answer panel, with the
// diff at the lines it is about; h there comes back to the list.
func (m *Model) openChat() tea.Cmd {
	if m.chIdx >= len(m.chats) {
		return nil
	}
	c := m.chats[m.chIdx]
	m.chatID = c.ID
	m.screen = screenDiff
	m.backTo = screenChats
	m.filesFocused = false
	m.overlay = overlayAnswer
	m.askScroll = -1
	fi := -1
	for i := range m.files {
		if m.files[i].Path() == c.Path {
			fi = i
			break
		}
	}
	if fi < 0 {
		return m.setStatus("File is no longer in the diff: "+c.Path, true)
	}
	cmd := m.selectFile(fi)
	for i := range m.rows {
		if o, n, ok := m.rows[i].nums(); ok && ((c.Side == "LEFT" && n == 0 && o == c.Line) || (c.Side != "LEFT" && n == c.Line)) {
			m.cursor = i
			break
		}
	}
	return cmd
}

// renderChats draws the conversations screen body.
func (m *Model) renderChats(width, height int) []string {
	title := styTitle.Render(fmt.Sprintf(" Claude (%d)", len(m.chats))) + styDim.Render("  conversations on this PR · "+m.keys.First(keys.Diff, "ask")+" in the diff starts one")
	lines := []string{padRight(truncate(title, width), width), styBorder.Render(strings.Repeat("─", width))}
	avail := height - len(lines)
	if avail < 1 {
		return lines[:min(len(lines), height)]
	}
	perPage := max(1, avail/chItemH)
	if m.chIdx < m.chScroll {
		m.chScroll = m.chIdx
	}
	if m.chIdx >= m.chScroll+perPage {
		m.chScroll = m.chIdx - perPage + 1
	}
	for i := m.chScroll; i < len(m.chats) && len(lines)+chItemH <= height; i++ {
		lines = append(lines, m.renderChatItem(&m.chats[i], i == m.chIdx, width)...)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines[:height]
}

// renderChatItem renders one conversation as chItemH lines.
func (m *Model) renderChatItem(c *state.Chat, selected bool, width int) []string {
	bg := color.Color(lipgloss.NoColor{})
	if selected {
		bg = colSelBg
	}
	base := lipgloss.NewStyle().Background(bg)
	info := fmt.Sprintf("  %d question(s) · %s", len(c.Entries), ago(c.UpdatedAt))
	l1 := " " + base.Foreground(colAccent).Render("✦ ") + base.Foreground(colText).Bold(true).Render(c.Loc) + base.Foreground(colDim).Render(info)
	first, last := "", ""
	if len(c.Entries) > 0 {
		first = oneLine(c.Entries[0].Q)
		last = oneLine(c.Entries[len(c.Entries)-1].A)
	}
	if a := m.waitingIn(c.ID); a != nil {
		if first == "" {
			first = oneLine(a.q)
		}
		last = "Claude is thinking…"
	}
	l2 := base.Render("   ") + base.Foreground(colWarn).Render("? "+first)
	l3 := base.Render("     ") + base.Foreground(colDim).Render(last)
	return []string{
		padRightBg(truncate(l1, width), width, bg),
		padRightBg(truncateTail(l2, width), width, bg),
		padRightBg(truncateTail(l3, width), width, bg),
		padRightBg("", width, bg),
	}
}

// oneLine collapses text to a single line for the list.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// chatAtY maps a screen row to a conversation index, or -1.
func (m *Model) chatAtY(y int) int {
	top := headerH + paneHeaderH
	if y < top {
		return -1
	}
	i := m.chScroll + (y-top)/chItemH
	if i >= len(m.chats) {
		return -1
	}
	return i
}
