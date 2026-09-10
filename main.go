// ghpr is a terminal UI for reviewing GitHub pull requests, built on
// bubbletea and the GitHub CLI.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"ghpr/internal/gh"
	"ghpr/internal/ui"
)

func usage() {
	fmt.Fprintf(os.Stderr, `ghpr – review GitHub pull requests in the terminal

Usage:
  ghpr [flags] [<number> | <url> | owner/repo#<number>]

Without a PR reference an interactive picker lists open pull requests.

Flags:
  -R, --repo owner/name   repository (default: repository of the current directory)
  -t, --theme name        chroma syntax theme (default: catppuccin-mocha, env GHPR_THEME)
  -s, --split             start in side-by-side mode
  -h, --help              show this help

Keys inside the app: press ? for the full list.
`)
}

func main() {
	var repo, theme string
	var split, help bool
	flag.StringVar(&repo, "R", "", "")
	flag.StringVar(&repo, "repo", "", "")
	flag.StringVar(&theme, "t", os.Getenv("GHPR_THEME"), "")
	flag.StringVar(&theme, "theme", os.Getenv("GHPR_THEME"), "")
	flag.BoolVar(&split, "s", false, "")
	flag.BoolVar(&split, "split", false, "")
	flag.BoolVar(&help, "h", false, "")
	flag.BoolVar(&help, "help", false, "")
	flag.Usage = usage
	flag.Parse()
	if help {
		usage()
		return
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

	model := ui.New(&gh.Client{Repo: repo}, number, theme)
	model.SetSplit(split)
	p := tea.NewProgram(model, tea.WithAltScreen())
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
