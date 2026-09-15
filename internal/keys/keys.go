// Package keys defines ghpr's key bindings and loads overrides from a TOML
// file. Bindings are grouped by context (the screen or menu they apply in)
// and named by action; each action may have several keys.
package keys

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Binding is one action in one context with its default keys.
type Binding struct {
	Ctx, Action string
	Keys        []string
	Desc        string
}

// Contexts, in the order they appear in the generated file.
const (
	List     = "list"     // pull request list
	Diff     = "diff"     // diff view
	Files    = "files"    // file panel focused (unmatched keys fall through to diff)
	Comments = "comments" // comments screen
	Notes    = "notes"    // notes screen
	Input    = "input"    // multi-line text entry
	Prompt   = "prompt"   // single-line prompts: search, file picker, all-files search, note
	Review   = "review"   // review menu
	Merge    = "merge"    // merge menu
	Pick     = "pick"     // choose a comment to delete/edit
	Confirm  = "confirm"  // close / reopen confirmation
)

var contextDesc = map[string]string{
	List:     "the pull request list",
	Diff:     "the diff view",
	Files:    "the file panel while it has focus; keys not listed here fall through to [diff]",
	Comments: "the comments screen (i)",
	Notes:    "the notes screen (A)",
	Input:    "multi-line text entry (comments, reviews, merge message)",
	Prompt:   "single-line prompts: / search, ctrl+p file picker, ctrl+/ search, note text. Printable keys type, so bind only control keys here",
	Review:   "the review menu (v)",
	Merge:    "the merge menu (M)",
	Pick:     "choosing one of your comments to delete (d) or edit (e)",
	Confirm:  "the close / reopen confirmation (X)",
}

// Defaults lists every binding with its default keys, in display order.
var Defaults = []Binding{
	{List, "quit", []string{"q"}, "quit"},
	{List, "cycle_state", []string{"s"}, "cycle open / closed / merged / all"},
	{List, "open", []string{"enter", "l", "right"}, "open the selected pull request"},

	{Diff, "quit", []string{"q"}, "back to the list when opened from it, otherwise quit"},
	{Diff, "force_quit", []string{"Q"}, "quit"},
	{Diff, "back", []string{"b", "backspace"}, "back to the pull request list"},
	{Diff, "down", []string{"j", "down"}, "move cursor down"},
	{Diff, "up", []string{"k", "up"}, "move cursor up"},
	{Diff, "half_page_down", []string{"ctrl+d", "pgdown", "space"}, "half page down"},
	{Diff, "half_page_up", []string{"ctrl+u", "pgup"}, "half page up"},
	{Diff, "page_down", []string{"ctrl+f"}, "full page down"},
	{Diff, "page_up", []string{"ctrl+b"}, "full page up"},
	{Diff, "top", []string{"g", "home"}, "top of the file"},
	{Diff, "bottom", []string{"G", "end"}, "bottom of the file"},
	{Diff, "next_file", []string{"]"}, "next file"},
	{Diff, "prev_file", []string{"["}, "previous file"},
	{Diff, "next_change", []string{"J"}, "next change in the file"},
	{Diff, "prev_change", []string{"K"}, "previous change in the file"},
	{Diff, "next", []string{"n"}, "next review thread, or next search match while a search is active"},
	{Diff, "prev", []string{"N"}, "previous review thread / search match"},
	{Diff, "scroll_right", []string{"l", "right"}, "scroll right when lines overflow"},
	{Diff, "scroll_left", []string{"h", "left"}, "scroll left; at the left edge go to the file tree (or back to the list you came from)"},
	{Diff, "focus", []string{"tab"}, "focus file list / diff"},
	{Diff, "toggle_files", []string{"f"}, "show / hide the file list"},
	{Diff, "tree_flat", []string{"t"}, "file list: tree / flat"},
	{Diff, "threads_only", []string{"T"}, "show only files with review threads"},
	{Diff, "split", []string{"s"}, "inline / side-by-side"},
	{Diff, "full_file", []string{"F"}, "full file view"},
	{Diff, "viewed", []string{"m"}, "mark / unmark the file as viewed"},
	{Diff, "select", []string{"V"}, "start / stop selecting lines"},
	{Diff, "cancel", []string{"esc"}, "cancel the selection, otherwise clear the search"},
	{Diff, "comment", []string{"c"}, "comment on the line or selection"},
	{Diff, "reply", []string{"r"}, "reply to the thread under the cursor"},
	{Diff, "resolve", []string{"x"}, "resolve / unresolve the thread under the cursor"},
	{Diff, "delete_comment", []string{"d"}, "delete one of your comments in the thread"},
	{Diff, "edit_comment", []string{"e"}, "edit one of your comments in the thread"},
	{Diff, "comments", []string{"i"}, "comments screen"},
	{Diff, "note", []string{"a"}, "add / remove a note on the line"},
	{Diff, "notes", []string{"A"}, "notes screen"},
	{Diff, "search", []string{"/"}, "search the file (file picker when the file list is focused)"},
	{Diff, "search_all", []string{"ctrl+/", "ctrl+_"}, "search every file"},
	{Diff, "file_picker", []string{"ctrl+p"}, "fuzzy file picker"},
	{Diff, "review", []string{"v"}, "submit a review"},
	{Diff, "pr_comment", []string{"C"}, "comment on the PR"},
	{Diff, "merge", []string{"M"}, "merge menu"},
	{Diff, "close_reopen", []string{"X"}, "close or reopen the PR"},
	{Diff, "editor", []string{"o"}, "open the file at the cursor line in Neovim"},
	{Diff, "browser", []string{"O"}, "open the PR in the browser"},
	{Diff, "refresh", []string{"R"}, "refresh PR, diff and threads"},
	{Diff, "help", []string{"?"}, "help"},

	{Files, "down", []string{"j", "down"}, "next entry"},
	{Files, "up", []string{"k", "up"}, "previous entry"},
	{Files, "top", []string{"g", "home"}, "first entry"},
	{Files, "bottom", []string{"G", "end"}, "last entry"},
	{Files, "open", []string{"enter", "space"}, "open the file / toggle the directory"},
	{Files, "expand", []string{"l", "right"}, "open the file / expand the directory"},
	{Files, "collapse", []string{"h", "left"}, "collapse the directory / go to the parent"},
	{Files, "collapse_all", []string{"H"}, "collapse every directory"},
	{Files, "expand_all", []string{"L"}, "expand every directory"},

	{Comments, "close", []string{"esc", "i", "q"}, "back to the diff"},
	{Comments, "down", []string{"j", "down"}, "next thread"},
	{Comments, "up", []string{"k", "up"}, "previous thread"},
	{Comments, "top", []string{"g", "home"}, "first thread"},
	{Comments, "bottom", []string{"G", "end"}, "last thread"},
	{Comments, "open", []string{"l", "right", "enter"}, "show the thread in the diff"},
	{Comments, "refresh", []string{"R"}, "refresh"},
	{Comments, "help", []string{"?"}, "help"},
	{Comments, "force_quit", []string{"Q"}, "quit"},

	{Notes, "close", []string{"esc", "A", "q"}, "back to the diff"},
	{Notes, "down", []string{"j", "down"}, "next note"},
	{Notes, "up", []string{"k", "up"}, "previous note"},
	{Notes, "top", []string{"g", "home"}, "first note"},
	{Notes, "bottom", []string{"G", "end"}, "last note"},
	{Notes, "open", []string{"l", "right", "enter"}, "go to the line in the diff"},
	{Notes, "delete", []string{"d", "x"}, "remove the note"},
	{Notes, "help", []string{"?"}, "help"},
	{Notes, "force_quit", []string{"Q"}, "quit"},

	{Input, "submit", []string{"ctrl+s", "super+j", "meta+j"}, "submit"},
	{Input, "cancel", []string{"esc"}, "cancel"},

	{Prompt, "accept", []string{"enter"}, "accept / open the selection"},
	{Prompt, "cancel", []string{"esc"}, "cancel"},
	{Prompt, "down", []string{"down", "ctrl+n", "ctrl+j", "tab"}, "next result"},
	{Prompt, "up", []string{"up", "ctrl+k", "shift+tab"}, "previous result"},
	{Prompt, "backspace", []string{"backspace"}, "delete the last character"},
	{Prompt, "clear", []string{"ctrl+u"}, "clear the query"},

	{Review, "approve", []string{"a"}, "approve"},
	{Review, "request_changes", []string{"r"}, "request changes"},
	{Review, "comment", []string{"c"}, "comment"},
	{Review, "cancel", []string{"esc", "v", "q"}, "cancel"},

	{Merge, "merge_commit", []string{"m"}, "merge commit (opens the message)"},
	{Merge, "squash", []string{"s"}, "squash (opens the message)"},
	{Merge, "rebase", []string{"r"}, "rebase (then confirm)"},
	{Merge, "delete_branch", []string{"d"}, "toggle deleting the branch"},
	{Merge, "confirm", []string{"y", "enter"}, "confirm a rebase merge"},
	{Merge, "cancel", []string{"esc", "n", "q", "M"}, "cancel"},

	{Pick, "down", []string{"j", "down"}, "next comment"},
	{Pick, "up", []string{"k", "up"}, "previous comment"},
	{Pick, "confirm", []string{"y", "enter"}, "delete / open the editor"},
	{Pick, "cancel", []string{"esc", "n", "q", "d", "e"}, "cancel"},

	{Confirm, "confirm", []string{"y", "enter"}, "confirm"},
	{Confirm, "delete_branch", []string{"d"}, "toggle deleting the branch (close only)"},
	{Confirm, "cancel", []string{"esc", "n", "q", "X"}, "cancel"},
}

// Unbound is returned by Translate for a default key whose action the user
// moved elsewhere; it matches no case in the handlers.
const Unbound = "\x00unbound"

// Map is the effective key map: per context, the keys bound to each action.
type Map struct {
	bound map[string]map[string][]string // ctx -> action -> keys
}

// Default returns the built-in bindings.
func Default() *Map {
	m := &Map{bound: map[string]map[string][]string{}}
	for _, b := range Defaults {
		if m.bound[b.Ctx] == nil {
			m.bound[b.Ctx] = map[string][]string{}
		}
		m.bound[b.Ctx][b.Action] = append([]string(nil), b.Keys...)
	}
	return m
}

// Load returns the defaults with the overrides in the TOML file at path
// applied. A missing file yields the defaults. Each entry replaces the
// action's whole key list; an empty list unbinds the action.
func Load(path string) (*Map, error) {
	m := Default()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := m.Apply(string(b)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Apply merges TOML overrides into the map.
func (m *Map) Apply(src string) error {
	var raw map[string]map[string]any
	if _, err := toml.Decode(src, &raw); err != nil {
		return err
	}
	for ctx, actions := range raw {
		if _, ok := m.bound[ctx]; !ok {
			return fmt.Errorf("unknown context [%s] (contexts: %s)", ctx, strings.Join(contextNames(), ", "))
		}
		for action, v := range actions {
			if _, ok := m.bound[ctx][action]; !ok {
				return fmt.Errorf("unknown action %s.%s (actions: %s)", ctx, action, strings.Join(m.actionNames(ctx), ", "))
			}
			var keys []string
			switch t := v.(type) {
			case string:
				keys = []string{t}
			case []any:
				for _, e := range t {
					s, ok := e.(string)
					if !ok {
						return fmt.Errorf("%s.%s: keys must be strings", ctx, action)
					}
					keys = append(keys, s)
				}
			default:
				return fmt.Errorf("%s.%s: expected a key or a list of keys", ctx, action)
			}
			m.bound[ctx][action] = keys
		}
	}
	// A key may serve only one action per context.
	for ctx, actions := range m.bound {
		seen := map[string]string{}
		for _, action := range sortedKeys(actions) {
			for _, k := range actions[action] {
				if other, dup := seen[k]; dup && other != action {
					return fmt.Errorf("[%s] key %q is bound to both %s and %s", ctx, k, other, action)
				}
				seen[k] = action
			}
		}
	}
	return nil
}

// Translate maps a pressed key to the canonical (first default) key of the
// action it is bound to in ctx, so handlers can keep matching on defaults.
// Keys bound to nothing come back unchanged, unless they are a default key
// whose action was rebound, which yields Unbound.
func (m *Map) Translate(ctx, key string) string {
	actions := m.bound[ctx]
	for action, keys := range actions {
		for _, k := range keys {
			if k == key {
				return defaultKey(ctx, action)
			}
		}
	}
	for _, b := range Defaults {
		if b.Ctx != ctx {
			continue
		}
		for _, k := range b.Keys {
			if k == key {
				return Unbound
			}
		}
	}
	return key
}

// Is reports whether key is bound to action in ctx.
func (m *Map) Is(ctx, action, key string) bool {
	for _, k := range m.bound[ctx][action] {
		if k == key {
			return true
		}
	}
	return false
}

// Keys returns the keys bound to an action.
func (m *Map) Keys(ctx, action string) []string { return m.bound[ctx][action] }

// First returns the primary key for an action, for hints; "" when unbound.
func (m *Map) First(ctx, action string) string {
	if k := m.bound[ctx][action]; len(k) > 0 {
		return Pretty(k[0])
	}
	return ""
}

// Label joins an action's keys for display, e.g. "j/↓"; "(unbound)" when
// nothing is bound.
func (m *Map) Label(ctx, action string) string {
	k := m.bound[ctx][action]
	if len(k) == 0 {
		return "(unbound)"
	}
	out := make([]string, len(k))
	for i, s := range k {
		out[i] = Pretty(s)
	}
	return strings.Join(out, "/")
}

// Pretty shortens key names for display.
func Pretty(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "pgdown":
		return "pgdn"
	}
	return k
}

func defaultKey(ctx, action string) string {
	for _, b := range Defaults {
		if b.Ctx == ctx && b.Action == action {
			return b.Keys[0]
		}
	}
	return action
}

func contextNames() []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range Defaults {
		if !seen[b.Ctx] {
			seen[b.Ctx] = true
			out = append(out, b.Ctx)
		}
	}
	return out
}

func (m *Map) actionNames(ctx string) []string {
	var out []string
	for _, b := range Defaults {
		if b.Ctx == ctx {
			out = append(out, b.Action)
		}
	}
	return out
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultTOML renders the default bindings as a commented TOML file, the
// starting point for a user's overrides.
func DefaultTOML() string {
	var sb strings.Builder
	sb.WriteString("# ghpr key bindings. Copy this file to the path shown by `ghpr --print-keys`\n")
	sb.WriteString("# and change the entries you want; anything left out keeps its default.\n")
	sb.WriteString("# Each action takes a list of keys (an empty list unbinds it). Key names are\n")
	sb.WriteString("# what `ghpr --debug-keys` shows: a, A, ctrl+p, shift+tab, enter, esc, up...\n")
	for _, ctx := range contextNames() {
		fmt.Fprintf(&sb, "\n# %s\n[%s]\n", contextDesc[ctx], ctx)
		for _, b := range Defaults {
			if b.Ctx != ctx {
				continue
			}
			quoted := make([]string, len(b.Keys))
			for i, k := range b.Keys {
				quoted[i] = fmt.Sprintf("%q", k)
			}
			fmt.Fprintf(&sb, "%-16s = [%s]  # %s\n", b.Action, strings.Join(quoted, ", "), b.Desc)
		}
	}
	return sb.String()
}
