# ghpr

Review GitHub pull requests in the terminal. `ghpr` is a Go TUI built on
[bubbletea v2](https://github.com/charmbracelet/bubbletea) (with bubbles v2 and
lipgloss v2, imported from their `charm.land/...` module paths) that uses the
[GitHub CLI](https://cli.github.com) (`gh`) for all data and actions, so it
reuses your existing `gh` login and works with any host `gh` is configured for.

## Features

- Syntax-highlighted diffs (via chroma), inline or side-by-side with a single-key toggle
- File list with per-file change counts and open/resolved thread badges
- Review threads rendered inline under the lines they belong to
- Create single-line and multi-line comments, reply to threads, resolve/unresolve threads
- Submit reviews: approve, request changes, or comment
- Post general PR comments
- Spinner-based loaders for every fetch and action, with success/error feedback
- Interactive picker of open PRs when no PR is given

## Requirements

- Go 1.25 or newer (to build)
- `gh` installed and authenticated: run `gh auth status` to check, `gh auth login` to sign in

## Install

```sh
git clone <this repo> ghpr
cd ghpr
make build          # produces ./ghpr
make install        # optional: moves it to /usr/local/bin (uses sudo)
```

Or with plain Go:

```sh
go build -o ghpr .
```

## Usage

```
ghpr [flags] [<number> | <url> | owner/repo#<number>]
```

Run inside a repository clone to review one of its PRs:

```sh
ghpr            # pick from open PRs
ghpr 42         # open PR #42
```

Or point at any repository:

```sh
ghpr -R owner/repo 42
ghpr owner/repo#42
ghpr https://github.com/owner/repo/pull/42
```

### Flags

| Flag | Description |
| --- | --- |
| `-R, --repo owner/name` | Repository to use (default: repository of the current directory) |
| `-t, --theme name` | Chroma syntax theme (default `catppuccin-mocha`, or `GHPR_THEME` env var) |
| `-s, --split` | Start in side-by-side mode |
| `-h, --help` | Show help |

Any chroma style name works for `--theme`, for example `dracula`, `github-dark`,
`monokai`, `nord`, `solarized-dark`.

## Key bindings

Press `?` inside the app for this list.

### Navigation

| Key | Action |
| --- | --- |
| `j` / `k`, `↓` / `↑` | Move cursor |
| `ctrl+d` / `ctrl+u`, `pgdn` / `pgup`, `space` | Half page down / up |
| `g` / `G` | Top / bottom of file |
| `]` / `[`, `l` / `h` | Next / previous file |
| `n` / `N` | Next / previous review thread (crosses files) |
| `tab` | Switch focus between file list and diff |
| `f` | Show / hide the file list |
| `s` | Toggle inline / side-by-side diff |

### Reviewing

| Key | Action |
| --- | --- |
| `c` | Comment on the line under the cursor, or on the selected range |
| `V` | Start selecting lines; move with `j`/`k`, then `c` to comment on the range, `esc` to cancel |
| `r` | Reply to the thread under the cursor |
| `x` | Resolve / unresolve the thread under the cursor |
| `v` | Submit a review, then `a` approve, `r` request changes, `c` comment |
| `C` | Post a general comment on the PR |
| `o` | Open the PR in the browser |
| `R` | Refresh PR, diff and threads |

### Text entry

| Key | Action |
| --- | --- |
| `⌘+enter` (also `ctrl+enter`) | Submit |
| `esc` | Cancel |

`enter` inserts a newline. An approval may be submitted with an empty body.
Every other comment or review needs text.

The Command key only reaches terminal apps when the terminal supports the
kitty keyboard protocol (Ghostty, kitty, WezTerm, recent iTerm2). Inside tmux,
add `set -s extended-keys on` to your tmux config so the modifier is passed
through. Where Command is not reported, use `ctrl+enter`.

### General

| Key | Action |
| --- | --- |
| `?` | Toggle help |
| `q`, `ctrl+c` | Quit |

## How it talks to GitHub

Every operation shells out to `gh`:

- `gh pr list`, `gh pr view`, `gh pr diff` for reading
- `gh pr review`, `gh pr comment` for reviews and PR comments
- `gh api` (REST) for creating line comments and replies
- `gh api graphql` for fetching review threads and resolving/unresolving them

## Development

```sh
make test                                  # unit tests
go test -tags live ./internal/gh -v        # read-only checks against a public repo (needs gh login)
make fmt
make lint                                  # requires golangci-lint
make build-all                             # cross-compile into dist/
```

Layout:

- `main.go` – flag parsing and program start
- `internal/gh` – `gh` wrapper: PR data, threads, comments, reviews
- `internal/diff` – unified diff parser and side-by-side pairing
- `internal/ui` – bubbletea model, rendering, syntax highlighting
