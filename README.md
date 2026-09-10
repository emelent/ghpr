# ghpr

Review GitHub pull requests in the terminal. `ghpr` is a Go TUI built on
[bubbletea v2](https://github.com/charmbracelet/bubbletea) (with bubbles v2 and
lipgloss v2, imported from their `charm.land/...` module paths) that uses the
[GitHub CLI](https://cli.github.com) (`gh`) for all data and actions, so it
reuses your existing `gh` login and works with any host `gh` is configured for.

## Features

- Syntax-highlighted diffs (via chroma), inline or side-by-side with a single-key toggle
- Full-file view that shows the whole file with the changes in place, and next/previous change navigation
- Mark files as viewed. Marks persist locally with a timestamp and are dropped automatically when a file changes after you viewed it
- File tree (or flat list) with per-file change counts and open/resolved thread badges; directories collapse and single-child paths are compacted
- Review threads rendered inline under the lines they belong to
- Create single-line and multi-line comments, reply to threads, resolve/unresolve threads
- Submit reviews: approve, request changes, or comment
- Merge the PR (merge commit, squash or rebase, optionally deleting the branch), editing the commit message first; the header shows the merge state
- Close and reopen PRs
- PR picker with text filter and open / closed / merged / all state filter
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

In the picker, `j`/`k` move, `/` filters by text, `s` cycles the state filter
(open → closed → merged → all; open is the default, or pass `--state`), and
`enter`, `l` or `→` opens the selected pull request. `q` quits.

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
| `-t, --theme name` | UI theme: `github-dark` (default) or `solarized-dark`. Env: `GHPR_THEME` |
| `--syntax name` | Chroma style for syntax highlighting. Defaults to the theme's pairing (`catppuccin-mocha` for github-dark, `solarized-dark` for solarized-dark). Env: `GHPR_SYNTAX` |
| `-s, --split` | Start in side-by-side mode |
| `-S, --state name` | Initial picker filter: `open` (default), `closed`, `merged` or `all` |
| `-V, --version` | Print the version and exit |
| `-h, --help` | Show help |

Any chroma style name works for `--syntax`, for example `dracula`, `github-dark`,
`monokai`, `nord`, `solarized-dark`. To use Solarized Dark everywhere:

```sh
ghpr -t solarized-dark 42
# or persist it
export GHPR_THEME=solarized-dark
```

## Key bindings

Press `?` inside the app for this list.

### Navigation

| Key | Action |
| --- | --- |
| `j` / `k`, `↓` / `↑` | Move cursor |
| `ctrl+d` / `ctrl+u`, `pgdn` / `pgup`, `space` | Half page down / up |
| `ctrl+f` / `ctrl+b` | Full page down / up |
| `g` / `G` | Top / bottom of the file, of the file list when it is focused, or of the PR list |
| `]` / `[`, `l` / `h` | Next / previous file |
| `J` / `K` | Next / previous change in the file |
| `n` / `N` | Next / previous review thread (crosses files) |
| `tab` | Switch focus between file list and diff |
| `f` | Show / hide the file list |
| `t` | File list as a directory tree (default) or a flat list |

When the file panel is focused in tree mode: `j`/`k` move through directories and files, `enter` or `space` toggles a directory (or opens a file and returns focus to the diff), `h`/`l` collapse / expand (`h` on a file jumps to its directory), `H`/`L` collapse / expand everything. Collapsed directories show file, viewed and change counts.
| `s` | Toggle inline / side-by-side diff |
| `m` | Mark / unmark the current file as viewed and move to the next file. Viewed files show a ✓ in the file list and the header shows when you viewed them |
| `F` | Toggle full-file view. Fetches the file at the PR head and shows every line with the hunks in place. Comments are still limited to lines that are part of the diff, as GitHub requires |

### Reviewing

| Key | Action |
| --- | --- |
| `c` | Comment on the line under the cursor, or on the selected range |
| `V` | Start selecting lines; move with `j`/`k`, then `c` to comment on the range, `esc` to cancel |
| `r` | Reply to the thread under the cursor |
| `x` | Resolve / unresolve the thread under the cursor |
| `v` | Submit a review, then `a` approve, `r` request changes, `c` comment |
| `M` | Merge the PR. Toggle `d` to delete the branch, then `m` merge commit or `s` squash opens the commit message (subject on the first line, body below) for editing; `⌘+s` merges. `r` rebase asks for `y` to confirm |
| `X` | Close the PR without merging (optionally deleting the branch), or reopen a closed PR, after a `y` confirmation |
| `C` | Post a general comment on the PR |
| `o` | Open the PR in the browser |
| `R` | Refresh PR, diff and threads |

### Text entry

| Key | Action |
| --- | --- |
| `⌘+s` (also `ctrl+s`) | Submit |
| `esc` | Cancel |

`enter` inserts a newline. An approval may be submitted with an empty body.
Every other comment or review needs text.

The Command key only reaches terminal apps when the terminal supports the
kitty keyboard protocol (Ghostty, kitty, WezTerm, recent iTerm2). Inside tmux,
add `set -s extended-keys on` to your tmux config so the modifier is passed
through. Where Command is not reported, use `ctrl+s`. Enter always inserts a
newline in the text box.

### General

| Key | Action |
| --- | --- |
| `?` | Toggle help |
| `b`, `backspace` | Back to the pull request list |
| `q` | Back to the list when the PR was opened from it, otherwise quit |
| `Q`, `ctrl+c` | Quit |

## Viewed files

Pressing `m` stores the file path, the time, the PR head commit and a
fingerprint of the file's diff in `viewed.json` under `$GHPR_STATE_DIR`, or the
user config directory (`~/Library/Application Support/ghpr` on macOS,
`~/.config/ghpr` on Linux) when the variable is unset. Each time the PR is
loaded or refreshed, files whose diff no longer matches the stored fingerprint,
meaning they changed after you viewed them, are unmarked and the status bar
says how many were reset. Marks are per pull request.

## How it talks to GitHub

Every operation shells out to `gh`:

- `gh pr list`, `gh pr view`, `gh pr diff` for reading
- `gh pr review`, `gh pr comment` for reviews and PR comments
- `gh pr merge`, `gh pr close`, `gh pr reopen` for merging, closing and reopening
- `gh api` (REST) for creating line comments and replies
- `gh api graphql` for fetching review threads and resolving/unresolving them

## Releases

Every push to `main` runs the release workflow (`.github/workflows/release.yml`):

1. Tests run.
2. The next version is derived from the commit messages since the last `v*` tag
   using [Conventional Commits](https://www.conventionalcommits.org):
   `feat!:` or a `BREAKING CHANGE` footer bumps major, `feat:` bumps minor,
   `fix:`/`perf:`/`refactor:`/`revert:` and non-conventional messages bump patch,
   and `chore:`/`docs:`/`ci:`/`test:`/`style:`/`build:` alone produce no release.
3. The commit is tagged, binaries are cross-compiled for Linux, macOS and Windows,
   and a GitHub release is published with archives, a `SHA256SUMS.txt` and
   auto-generated notes.

Preview the next tag locally with `.github/scripts/next-version.sh`. Pull
requests run `gofmt`, `go vet`, tests and a build via `.github/workflows/ci.yml`.

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
