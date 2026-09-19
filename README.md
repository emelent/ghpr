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
- Resumes at the last file and line you were viewing when you reopen a PR
- File tree (or flat list) with per-file change counts and open/resolved thread badges; directories collapse and single-child paths are compacted
- Review threads rendered inline under the lines they belong to
- Create single-line and multi-line comments, reply to threads, resolve/unresolve threads, delete your own comments
- Submit reviews: approve, request changes, or comment. The PR header and the picker show who has approved (✓) and who has requested changes (✗), from each reviewer's latest review
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
| `--debug-keys` | Show the name of every key press in the status bar |
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
| `]` / `[` | Next / previous file |
| `ctrl+p`, `/` in the file list | Fuzzy-find a file. Type part of the path (spaces are ignored, so `cmd main` finds `cmd/x/main.go`), `↑`/`↓` or `ctrl+j`/`ctrl+k` choose, `enter` opens, `esc` cancels |
| `l` | Scroll the diff right when lines are wider than the pane; in the file tree, open the selected file |
| `h` | Scroll the diff back left; at the left edge, move focus to the file tree |
| `J` / `K` | Next / previous change in the file |
| `n` / `N` | Next / previous review thread (crosses files); next / previous match while a search is active |
| `/` | Search the current file, ignoring case. The cursor follows as you type; `enter` keeps the match, `esc` goes back. Hits are highlighted and `n` / `N` step through them; `esc` clears the search |
| `ctrl+/` | Search every file in the diff, ignoring case. Lines containing the text exactly are listed first, then fuzzy hits where the characters appear in order (so `fprint` finds `fmt.Println`). Results show `path:line` and the matching text; `↑`/`↓` or `ctrl+j`/`ctrl+k` choose, `enter` jumps to that line, `esc` cancels. After jumping to an exact hit the query stays active as the in-file search so `n` / `N` work there |
| `tab` | Switch focus between file list and diff |
| `f` | Show / hide the file list |
| `t` | File list as a directory tree (default) or a flat list |
| `T` | Show only files that have review threads. The file list, `]` / `[`, `ctrl+p` and `ctrl+/` all follow the filter; press again to see every file |
| `i` | Comments screen: every open review thread with its file and line, the code line it points at and the first comment, ordered by file. Resolved threads are hidden; `t` shows them too. `j`/`k` move, `l` or `enter` opens the thread in the diff, `h` there comes back to the list, `r` replies to the selected thread in a panel below the list, `x` resolves or unresolves it, `/` filters the list by file path, comment author or comment text (the matching comment is the one shown, `esc` clears the filter), `esc` closes it. Click a thread to select it, click again to open |
| `a` | Add a note on the line under the cursor, for something to come back to. You are asked for the text (what you wanted to do there); `enter` saves, `esc` cancels. Lines with a note show amber line numbers and the note in the status bar; `a` on such a line removes the note. Notes are saved per pull request alongside the viewed marks, so they are there again when you reopen the PR |
| `A` | Notes screen: every note with its file, line and code. `j`/`k` move, `l` or `enter` jumps to the line in the diff, `h` there comes back to the list, `d` removes a note, `esc` closes |

The mouse works too: click a line to move the cursor there, drag (or `shift`+click) to select a range for a multi-line comment, and use the wheel to scroll. In the file list, click a file to open it or a folder to fold / unfold it, and the wheel moves through the list. In the pull request list, click a PR to select it and click it again to open it.

When the file panel is focused in tree mode: `j`/`k` move through directories and files, `enter`, `space` or `l` on a file opens it and returns focus to the diff; on a directory `enter`/`space` toggle it, `l` expands and `h` collapses (`h` on a file jumps to its directory), `H`/`L` collapse / expand everything. From the diff, `h` brings you back to the tree. Collapsed directories show file, viewed and change counts.
| `s` | Toggle inline / side-by-side diff |
| `m` | Mark / unmark the current file as viewed. With a folder selected in the file tree, marks every file beneath it (or unmarks them all when they all are). Marking moves focus to the file tree so you can pick the next file. When every file in a folder is viewed the folder folds, and so do its parents up to the first folder with unviewed files. Fully viewed folders start folded when you open the PR. Viewed files show a ✓ in the file list and the header shows when you viewed them. Files you ticked as viewed on github.com are pulled in when the PR opens, and `m` pushes your mark back to GitHub |
| `F` | Toggle full-file view. Fetches the file at the PR head and shows every line with the hunks in place. Comments are still limited to lines that are part of the diff, as GitHub requires |

### Reviewing

| Key | Action |
| --- | --- |
| `c` | Comment on the line under the cursor, or on the selected range |
| `V` | Start selecting lines; move with `j`/`k`, then `c` to comment on the range, `esc` to cancel |
| `r` | Reply to the thread under the cursor |
| `x` | Resolve / unresolve the thread under the cursor |
| `d` | Delete one of your comments in the thread under the cursor. The newest is preselected; `j`/`k` pick another, `y` confirms |
| `e` | Edit one of your comments in the thread under the cursor. Pick it like `d`, then `y` opens the editor pre-filled with the current text; `ctrl+s` saves |
| `v` | Submit a review, then `a` approve, `r` request changes, `c` comment |
| `M` | Merge the PR. Toggle `d` to delete the branch, then `m` merge commit or `s` squash opens the commit message (subject on the first line, body below) for editing; `ctrl+s` merges. `r` rebase asks for `y` to confirm |
| `X` | Close the PR without merging (optionally deleting the branch), or reopen a closed PR, after a `y` confirmation |
| `C` | Post a general comment on the PR |
| `o` | Open the current file at the cursor's line in Neovim (see below); does nothing unless an editor is listening for this PR |
| `O` | Open the PR in the browser |
| `R` | Refresh PR, diff and threads |

### Editing alongside Neovim

Start Neovim in the repository with a server socket named after the pull
request's GraphQL node id (not its number), in a tmux window called `code` in
the same session as `ghpr`:

```sh
id=$(gh pr view 123 --json id -q .id)      # e.g. PR_kwDOPRY-OM8AAAABCBHOJc
nvim --listen /tmp/nvim.$id.sock
```

Pressing `o` while reviewing that PR runs `nvim --server /tmp/nvim.$id.sock
--remote-expr "execute('edit +LINE ' . fnameescape('./path/to/file'))"` for
the file under the cursor, where `LINE` is the cursor's line in the new version
of the file (for a removed line, the nearest line that still exists), and
switches the tmux session to the `code` window. An expression is used rather
than sent keys so your mappings and the current mode cannot interfere. When
the socket does not exist nothing happens; outside tmux only the file is sent.

### Custom key bindings

Every key above can be changed in `keys.toml`, kept in the same directory as
the viewed-file state (`$GHPR_STATE_DIR` or `<user config dir>/ghpr`). Print
the defaults, save them there and edit the entries you want; anything left out
keeps its default:

```sh
ghpr --print-keys > "$(ghpr --print-keys 2>&1 >/dev/null | sed 's/^# save as //')"
```

Bindings are grouped by context: `[list]`, `[diff]`, `[files]` (file panel
focused), `[comments]`, `[notes]`, `[input]`, `[prompt]`, `[review]`, `[merge]`,
`[pick]` and `[confirm]`. Each action takes a list of key names as
`--debug-keys` shows them; an empty list unbinds the action:

```toml
[diff]
next_file = ["n", "ctrl+n"]   # n moves to the next file…
next      = ["ctrl+j"]        # …so the thread / match jump needs a new key
notes     = []                # unbind

[prompt]
down = ["ctrl+n"]
up   = ["ctrl+p"]
```

A key may serve only one action per context; conflicts and unknown names are
reported at start-up. The help screen (`?`) and the status bar hints show the
bindings in effect. `ctrl+c` always quits.

### Text entry

| Key | Action |
| --- | --- |
| `ctrl+s` | Submit |
| `esc` | Cancel |

`enter` inserts a newline. An approval may be submitted with an empty body.
Every other comment or review needs text.

`⌘+j` also submits in terminals that speak the kitty keyboard protocol
(Ghostty, kitty, WezTerm, recent iTerm2), which is the only way the Command key
reaches terminal apps. Inside tmux, enable extended keys so the protocol is
passed through:

```
set -s extended-keys on
set -as terminal-features 'xterm*:extkeys'
```

Run `ghpr --debug-keys ...` to see the name of every key your terminal sends
in the status bar.

### General

| Key | Action |
| --- | --- |
| `?` | Toggle help |
| `b`, `backspace` | Back to the pull request list |
| `q` | Back to the list when the PR was opened from it, otherwise quit |
| `Q`, `ctrl+c` | Quit |

## Viewed files and resume

The last file and line you were on in each pull request are remembered, and
reopening the PR takes you straight back there (the status bar says
`Resumed at <file>`). The position is saved when you change file, go back to
the list, or quit.

Pressing `m` stores the file path, the time, the PR head commit and a
fingerprint of the file's diff in `viewed.json` under `$GHPR_STATE_DIR`, or the
user config directory (`~/Library/Application Support/ghpr` on macOS,
`~/.config/ghpr` on Linux) when the variable is unset. Each time the PR is
loaded or refreshed, files whose diff no longer matches the stored fingerprint,
meaning they changed after you viewed them, are unmarked and the status bar
says how many were reset. Marks are per pull request.

## How it talks to GitHub

Every operation shells out to `gh`:

- `gh pr list`, `gh pr view`, `gh pr diff` for reading; when a PR has more than
  300 files GitHub refuses the diff, so the per-file REST API
  (`pulls/{n}/files`, up to 3000 files) is used instead. Patches GitHub omits
  as too large show a note; `F` still loads the full file
- `gh pr review`, `gh pr comment` for reviews and PR comments
- `gh pr merge`, `gh pr close`, `gh pr reopen` for merging, closing and reopening
- `gh api` (REST) for creating, editing and deleting line comments and replies
- `gh api graphql` for fetching review threads and resolving/unresolving them,
  and for the "Viewed" ticks on files: they are pulled when a PR opens and
  merged into the local marks, and `m` pushes yours back so github.com agrees

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
- `internal/editor` – hands files to a listening Neovim and switches tmux windows
- `internal/ui` – bubbletea model, rendering, syntax highlighting
