// ghpr is a terminal UI for reviewing GitHub pull requests, built on
// bubbletea and the GitHub CLI.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"ghpr/internal/gh"
	"ghpr/internal/state"
	"ghpr/internal/ui"
)

// version is injected at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func usage() {
	fmt.Fprintf(os.Stderr, `ghpr – review GitHub pull requests in the terminal

Usage:
  ghpr [flags] [<number> | <url> | owner/repo#<number>]

Without a PR reference an interactive picker lists open pull requests.
Viewed-file marks are stored in $GHPR_STATE_DIR or the user config dir (ghpr/viewed.json).

Flags:
  -R, --repo owner/name   repository (default: repository of the current directory)
  -t, --theme name        UI theme: %s (default: %s, env GHPR_THEME)
  --syntax name           chroma style for syntax highlighting (default: the theme's own, env GHPR_SYNTAX)
  -s, --split             start in side-by-side mode
  -S, --state name        PR picker filter: open (default), closed, merged, all
  --debug-keys            show the name of every key press in the status bar
  -V, --version           print the version and exit
  -h, --help              show this help

Keys inside the app: press ? for the full list.
`, strings.Join(ui.ThemeNames(), ", "), ui.DefaultTheme)
}

func main() {
	var repo, theme, syntax, listState string
	var split, help, showVersion, debugKeys bool
	flag.StringVar(&repo, "R", "", "")
	flag.StringVar(&repo, "repo", "", "")
	flag.StringVar(&theme, "t", os.Getenv("GHPR_THEME"), "")
	flag.StringVar(&theme, "theme", os.Getenv("GHPR_THEME"), "")
	flag.StringVar(&syntax, "syntax", os.Getenv("GHPR_SYNTAX"), "")
	flag.StringVar(&listState, "S", "open", "")
	flag.StringVar(&listState, "state", "open", "")
	flag.BoolVar(&split, "s", false, "")
	flag.BoolVar(&split, "split", false, "")
	flag.BoolVar(&debugKeys, "debug-keys", false, "")
	flag.BoolVar(&showVersion, "V", false, "")
	flag.BoolVar(&showVersion, "version", false, "")
	flag.BoolVar(&help, "h", false, "")
	flag.BoolVar(&help, "help", false, "")
	flag.Usage = usage
	flag.Parse()
	if help {
		usage()
		return
	}
	if showVersion {
		fmt.Println("ghpr " + version)
		return
	}

	if err := ui.ApplyTheme(theme); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if syntax == "" {
		t, _ := ui.LookupTheme(theme)
		if theme == "" {
			t, _ = ui.LookupTheme(ui.DefaultTheme)
		}
		syntax = t.Syntax
	}

	number := 0
	if flag.NArg() > 0 {
		r, n, err := gh.ParsePRRef(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		number = n
		if r != "" && repo == "" {
			repo = r
		}
	}
	if repo == "" {
		r, err := gh.CurrentRepo()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot determine repository; pass -R owner/name or a PR URL")
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		repo = r
	}

	model := ui.New(&gh.Client{Repo: repo}, number, syntax)
	model.SetSplit(split)
	model.SetListState(listState)
	model.SetDebugKeys(debugKeys)
	if dir, err := state.Dir(); err == nil {
		if st, err := state.Open(dir); err == nil {
			model.SetStore(st)
		} else {
			fmt.Fprintln(os.Stderr, "warning: viewed-file state disabled:", err)
		}
	}
	p := tea.NewProgram(model)
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if fm, ok := final.(*ui.Model); ok && fm.Fatal() != nil {
		fmt.Fprintln(os.Stderr, fm.Fatal())
		os.Exit(1)
	}
}
