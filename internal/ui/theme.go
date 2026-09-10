package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme is a complete UI colour palette plus the chroma style that pairs
// with it for syntax highlighting.
type Theme struct {
	Name   string
	Syntax string // default chroma style name

	AppBg, PanelBg, BarBg, Border string
	Text, Dim, NumFg              string
	Accent, Warn, OK, Err         string

	AddFg, DelFg                     string
	AddBg, DelBg                     string // plain diff lines
	AddCurBg, DelCurBg, CtxCurBg     string // row under the cursor
	AddSelBg, DelSelBg, CtxSelBg     string // rows in a visual selection
	FileSelBg, ThreadBg, ThreadCurBg string
}

// DefaultTheme is used when no theme is requested.
const DefaultTheme = "github-dark"

var themes = map[string]Theme{
	"github-dark": {
		Name: "github-dark", Syntax: "catppuccin-mocha",
		AppBg: "#0d1117", PanelBg: "#161b22", BarBg: "#21262d", Border: "#30363d",
		Text: "#e6edf3", Dim: "#8b949e", NumFg: "#6e7681",
		Accent: "#79c0ff", Warn: "#d29922", OK: "#3fb950", Err: "#f85149",
		AddFg: "#3fb950", DelFg: "#f85149",
		AddBg: "#12331f", DelBg: "#3b1418",
		AddCurBg: "#1f5a35", DelCurBg: "#6b2027", CtxCurBg: "#2c3140",
		AddSelBg: "#1a4a3a", DelSelBg: "#5a2a3a", CtxSelBg: "#23304a",
		FileSelBg: "#264f78", ThreadBg: "#0f141a", ThreadCurBg: "#1e2733",
	},
	// Ethan Schoonover's Solarized Dark: base03 background, base0/base1
	// text, accents from the canonical eight hues. Diff backgrounds are the
	// base03 background blended toward green/red.
	"solarized-dark": {
		Name: "solarized-dark", Syntax: "solarized-dark",
		AppBg: "#002b36", PanelBg: "#073642", BarBg: "#073642", Border: "#586e75",
		Text: "#eee8d5", Dim: "#839496", NumFg: "#586e75",
		Accent: "#268bd2", Warn: "#b58900", OK: "#859900", Err: "#dc322f",
		AddFg: "#859900", DelFg: "#dc322f",
		AddBg: "#173f2a", DelBg: "#3a2a33",
		AddCurBg: "#2f5a22", DelCurBg: "#632e33", CtxCurBg: "#0c4a5a",
		AddSelBg: "#254c28", DelSelBg: "#4d2d34", CtxSelBg: "#083c4a",
		FileSelBg: "#1a5f8a", ThreadBg: "#00212b", ThreadCurBg: "#073642",
	},
}

// ThemeNames lists the available UI themes, sorted.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for n := range themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LookupTheme returns a theme by name (case-insensitive).
func LookupTheme(name string) (Theme, bool) {
	t, ok := themes[strings.ToLower(strings.TrimSpace(name))]
	return t, ok
}

// ApplyTheme installs the named theme's colours into the package palette.
// It must be called before constructing a Model.
func ApplyTheme(name string) error {
	if name == "" {
		name = DefaultTheme
	}
	t, ok := LookupTheme(name)
	if !ok {
		return fmt.Errorf("unknown theme %q (available: %s)", name, strings.Join(ThemeNames(), ", "))
	}
	c := lipgloss.Color
	colAppBg, colPanelBg, colBarBg, colBorder = c(t.AppBg), c(t.PanelBg), c(t.BarBg), c(t.Border)
	colText, colDim, colNumFg = c(t.Text), c(t.Dim), c(t.NumFg)
	colAccent, colWarn, colOK, colErr = c(t.Accent), c(t.Warn), c(t.OK), c(t.Err)
	colAddFg, colDelFg = c(t.AddFg), c(t.DelFg)
	colAddBg, colDelBg = c(t.AddBg), c(t.DelBg)
	colAddCurBg, colDelCurBg, colCtxCurBg = c(t.AddCurBg), c(t.DelCurBg), c(t.CtxCurBg)
	colAddSelBg, colDelSelBg, colCtxSelBg = c(t.AddSelBg), c(t.DelSelBg), c(t.CtxSelBg)
	colSelBg, colThreadBg, colThreadCu = c(t.FileSelBg), c(t.ThreadBg), c(t.ThreadCurBg)
	rebuildStyles()
	return nil
}
