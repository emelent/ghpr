package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Colour tokens and derived styles. They are populated by ApplyTheme; the
// default theme is applied at package init so tests and callers that never
// pick a theme still get a complete palette.
var (
	colAddBg, colDelBg, colAddCurBg, colDelCurBg, colCtxCurBg color.Color
	colAddSelBg, colDelSelBg, colCtxSelBg                     color.Color
	colAddFg, colDelFg, colNumFg, colDim, colAccent           color.Color
	colWarn, colOK, colErr, colText                           color.Color
	colPanelBg, colBarBg, colSelBg, colThreadBg, colThreadCu  color.Color
	colBorder, colAppBg, colMatchBg, colMatchFg               color.Color

	styTitle, styDim, styAccent, styWarn, styOK, styErr          lipgloss.Style
	styBar, styBarKey, styBarDim, styHunk, styHunkCur, styBorder lipgloss.Style
	styFileSel, styFileSelD, styNote, styInputTtl, styHelpKey    lipgloss.Style
)

func init() {
	if err := ApplyTheme(DefaultTheme); err != nil {
		panic(err)
	}
}

// rebuildStyles derives the lipgloss styles from the current colour tokens.
func rebuildStyles() {
	styTitle = lipgloss.NewStyle().Bold(true).Foreground(colText)
	styDim = lipgloss.NewStyle().Foreground(colDim)
	styAccent = lipgloss.NewStyle().Foreground(colAccent)
	styWarn = lipgloss.NewStyle().Foreground(colWarn)
	styOK = lipgloss.NewStyle().Foreground(colOK)
	styErr = lipgloss.NewStyle().Foreground(colErr)
	styBar = lipgloss.NewStyle().Background(colBarBg).Foreground(colText)
	styBarKey = lipgloss.NewStyle().Background(colBarBg).Foreground(colAccent).Bold(true)
	styBarDim = lipgloss.NewStyle().Background(colBarBg).Foreground(colDim)
	styHunk = lipgloss.NewStyle().Foreground(colAccent).Background(colPanelBg)
	styHunkCur = lipgloss.NewStyle().Foreground(colAccent).Background(colCtxCurBg)
	styBorder = lipgloss.NewStyle().Foreground(colBorder)
	styFileSel = lipgloss.NewStyle().Background(colSelBg).Foreground(colText).Bold(true)
	styFileSelD = lipgloss.NewStyle().Background(colCtxCurBg).Foreground(colText)
	styNote = lipgloss.NewStyle().Foreground(colDim).Italic(true)
	styInputTtl = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	styHelpKey = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
}

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
