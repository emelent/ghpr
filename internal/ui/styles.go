package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	colAddBg    = lipgloss.Color("#12331f")
	colDelBg    = lipgloss.Color("#3b1418")
	colAddCurBg = lipgloss.Color("#1f5a35")
	colDelCurBg = lipgloss.Color("#6b2027")
	colCtxCurBg = lipgloss.Color("#2c3140")
	colAddSelBg = lipgloss.Color("#1a4a3a")
	colDelSelBg = lipgloss.Color("#5a2a3a")
	colCtxSelBg = lipgloss.Color("#23304a")
	colAddFg    = lipgloss.Color("#3fb950")
	colDelFg    = lipgloss.Color("#f85149")
	colNumFg    = lipgloss.Color("#6e7681")
	colDim      = lipgloss.Color("#8b949e")
	colAccent   = lipgloss.Color("#79c0ff")
	colWarn     = lipgloss.Color("#d29922")
	colOK       = lipgloss.Color("#3fb950")
	colErr      = lipgloss.Color("#f85149")
	colText     = lipgloss.Color("#e6edf3")
	colPanelBg  = lipgloss.Color("#161b22")
	colBarBg    = lipgloss.Color("#21262d")
	colSelBg    = lipgloss.Color("#264f78")
	colThreadBg = lipgloss.Color("#0f141a")
	colThreadCu = lipgloss.Color("#1e2733")
	colBorder   = lipgloss.Color("#30363d")
	colAppBg    = lipgloss.Color("#0d1117")

	styTitle    = lipgloss.NewStyle().Bold(true).Foreground(colText)
	styDim      = lipgloss.NewStyle().Foreground(colDim)
	styAccent   = lipgloss.NewStyle().Foreground(colAccent)
	styWarn     = lipgloss.NewStyle().Foreground(colWarn)
	styOK       = lipgloss.NewStyle().Foreground(colOK)
	styErr      = lipgloss.NewStyle().Foreground(colErr)
	styBar      = lipgloss.NewStyle().Background(colBarBg).Foreground(colText)
	styBarKey   = lipgloss.NewStyle().Background(colBarBg).Foreground(colAccent).Bold(true)
	styBarDim   = lipgloss.NewStyle().Background(colBarBg).Foreground(colDim)
	styHunk     = lipgloss.NewStyle().Foreground(colAccent).Background(colPanelBg)
	styHunkCur  = lipgloss.NewStyle().Foreground(colAccent).Background(colCtxCurBg)
	styBorder   = lipgloss.NewStyle().Foreground(colBorder)
	styFileSel  = lipgloss.NewStyle().Background(colSelBg).Foreground(colText).Bold(true)
	styFileSelD = lipgloss.NewStyle().Background(colCtxCurBg).Foreground(colText)
	styNote     = lipgloss.NewStyle().Foreground(colDim).Italic(true)
	styInputTtl = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	styHelpKey  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
)

// truncate cuts an ANSI string to a display width without a tail.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "")
}

// truncateTail cuts with an ellipsis.
func truncateTail(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// padRight pads/truncates an ANSI string to exactly w cells.
func padRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw > w {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", w-sw)
}

// padRightBg pads with a background-styled filler.
func padRightBg(s string, w int, bg color.Color) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw > w {
		return truncate(s, w)
	}
	if sw == w {
		return s
	}
	return s + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", w-sw))
}

// leftEllipsis shortens a path keeping its tail visible.
func leftEllipsis(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	if w == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-(w-1):])
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func digits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}
