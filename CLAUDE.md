# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ghpr` is a Go terminal UI (bubbletea v2) for reviewing GitHub pull requests. Every GitHub operation shells out to the `gh` CLI; there is no direct HTTP client. Module path is `ghpr`, Go 1.25.

## Commands

```sh
make build                      # go build -> ./ghpr (version from git describe)
make test                       # go test ./...
go test ./internal/ui -run TestSearch          # one test
go test ./internal/ui -run 'TestLineNotes|TestCommentsScreen'
go test -tags live ./internal/gh -v            # read-only checks against a public repo; needs gh login
make fmt && go vet ./...        # CI runs gofmt -l, go vet, tests and a build (.github/workflows/ci.yml)
make lint                       # golangci-lint, optional locally
go run . --print-keys           # dump default key bindings as TOML
go run . --debug-keys 123       # show the name of each key press in the status bar
```

gofmt and vet must be clean; CI fails otherwise. `go vet -tags live ./internal/gh` currently fails in `live_content_test.go` (stale `ListPRs` call); that file is not part of the normal run.

**Commit messages drive releases.** Every push to `main` tags and publishes a release, with the version bump derived from Conventional Commit prefixes: `feat!`/`BREAKING CHANGE` major, `feat:` minor, `fix:`/`perf:`/`refactor:` and unprefixed messages patch, `chore:`/`docs:`/`ci:`/`test:` no release. Preview with `.github/scripts/next-version.sh`.

## Architecture

```
main.go              flags, theme/keys/state loading, tea.NewProgram
internal/gh          gh CLI wrapper: run() -> exec gh; REST via `gh api`, GraphQL via `gh api graphql`
internal/diff        unified diff parser (Parse, ParseHunks), side-by-side pairing, Expand for full-file view
internal/ui          the whole TUI: one Model, many files split by feature
internal/keys        key bindings: defaults table, TOML overrides, Translate()
internal/state       JSON store (viewed.json): viewed marks, last position, notes; keyed "owner/repo#N"
internal/editor      Neovim hand-off (--remote-expr) + tmux window switch
```

### The single Model (`internal/ui/model.go`)

One `Model` holds everything. Two orthogonal state axes decide what a key or mouse event does:

- `screen`: `screenPicker` (PR list, a bubbles `list.Model`), `screenDiff`, `screenComments`, `screenNotes`. Note `screenPicker` is the zero value; never test "came from a list" with `!= screenDiff`, use `hasBack()`.
- `overlay` (diff screen only): `overlayInput` (textarea for comments/reviews/merge message), `overlayReview`, `overlayMerge`, `overlayState` (close/reopen), `overlayDelete`/`overlayEdit` (comment picker), `overlaySearch`, `overlayFiles` (fuzzy file picker), `overlayGlobal` (all-files search), `overlayNote`, `overlayHelp`.

`Update` handles messages; `handleKey` dispatches by screen, then overlay, then file-panel focus, then the diff switch. `view()` composes header + body + status bar; overlays either replace the right pane (help, file picker, global search) or sit below the diff (input).

### Rows: the diff view's unit

`rebuildRows` flattens the current file into `m.rows []row` via `buildRows` (rows.go): hunk headers, lines (or side-by-side pairs), review threads interleaved under their anchor line, notes. `m.rowStart`/`rowHeight` give vertical offsets because thread rows are multi-line. The cursor is a row index; scrolling follows the cursor (`ensureCursorVisible` at render time). Anything that needs "which line is this" goes through `row.nums()` (old,new numbers) or `row.anchor()`. Line identity across view modes and reloads is by (oldNum,newNum), never by `*diff.Line` pointer, because full-file mode builds new Line values.

Syntax highlighting is chroma spans per `*diff.Line` (`highlight.go`), cached per file; search hits are injected by splitting spans (`markRanges`) so the renderer only needs the `Match` flag.

### Key bindings (`internal/keys`)

Handlers still `switch` on the *default* key names (`case "j", "down"`). `handleKey` first calls `m.keys.Translate(ctx, key)`, which maps whatever the user bound to the canonical first default key of that action, or to `keys.Unbound` for a default key the user moved away. When adding a key: add a `Binding` to `keys.Defaults` for the right context, match on its first default key in the handler, and add a `helpRow` in `renderHelp`. Status-bar hints use `m.hk(ctx, action)`. Bubbletea key names are lowercase (`f1`, `ctrl+p`); legacy terminals send ctrl+/ as `ctrl+_`.

### Async work

`m.action(label, refresh, fn)` runs fn in a tea.Cmd with a spinner and returns `actionMsg`; `refresh=true` reloads PR and threads afterwards. Loads use `pending` counting (`loadAll`, `loadDone`). Status messages via `setStatus` auto-clear after 5s and take precedence in the bar over passive info (active search, note text).

### Layout constants used by the mouse

`headerH` (2 PR header lines) and `paneHeaderH` (2 pane title/border lines) in mouse.go. Click mapping for the diff walks `rowStart`; for the PR list it uses the bubbles list styles and delegate height stored at construction; comments/notes screens have per-item heights.

### Viewed marks, notes, GitHub sync

Viewed marks are keyed by file fingerprint and dropped when the diff changes (`reconcileViewed`). On load, GitHub's "Viewed" ticks are pulled (GraphQL `viewerViewedState`) and merged one-way; `m` pushes with `markFileAsViewed`/`unmarkFileAsViewed` using the PR node ID (`pr.ID`). Notes (`a`/`A`) persist in the same store. The Neovim socket is `/tmp/nvim.<pr node id>.sock`, not the PR number.

### Large PRs

`gh pr diff` fails with HTTP 406 over 300 files; `Client.Files` falls back to the paginated `pulls/{n}/files` REST API and rebuilds `diff.File`s (`filesFromAPI`), flagging `PatchOmitted` where GitHub drops the patch.

## Testing conventions

`internal/ui/model_test.go` has `newTestModel(t)`: a 120x30 model with the `sample` diff (main.go, b.py) and three threads. Row indices in tests rely on that fixture: rows 0 hunk, 3 `-import "fmt"`, 4 thread T2, 6 `+"fmt"`, 7 thread T1, 11 `fmt.Println`. Tests drive `handleKey` with `tea.KeyPressMsg{Code: rune, Text: k}` (control keys: `Mod: tea.ModCtrl`) and assert on `ansi.Strip(m.View().Content)`. `treeSample` provides nested directories. Stores in tests come from `state.Open(t.TempDir())`. Package-level function variables (`editorHasServer`, `editorOpen`, `keys` on the model) exist so tests can stub externals; nothing in the unit tests shells out to `gh`.
