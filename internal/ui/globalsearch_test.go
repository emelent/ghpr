package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestGlobalMatch(t *testing.T) {
	cases := []struct {
		text, q string
		want    [][2]int
		exact   bool
	}{
		{`fmt.Println("hi")`, "fmt", [][2]int{{0, 3}}, true},                // exact wins
		{`fmt.Println("hi")`, "PRINTLN", [][2]int{{4, 11}}, true},           // case ignored
		{`fmt.Println("hi")`, "fprint", [][2]int{{0, 1}, {4, 9}}, false},    // fuzzy: f + Print, adjacent runs merged
		{`fmt.Println("hi")`, "fmt print", [][2]int{{0, 3}, {4, 9}}, false}, // spaces skipped in fuzzy mode
		{`fmt.Println("hi")`, "FP", [][2]int{{0, 1}, {4, 5}}, false},        // fuzzy ignores case too
		{`fmt.Println("hi")`, "xyz", nil, false},                            // characters absent
		{`fmt.Println("hi")`, "tmf", nil, false},                            // out of order
		{"héllo wörld", "hw", [][2]int{{0, 1}, {6, 7}}, false},              // rune offsets
	}
	for _, c := range cases {
		got, exact := globalMatch(c.text, c.q)
		if !reflect.DeepEqual(got, c.want) || exact != c.exact {
			t.Errorf("globalMatch(%q, %q) = %v,%v want %v,%v", c.text, c.q, got, exact, c.want, c.exact)
		}
	}
	// Fuzzy hits go through the same span marking as exact ones.
	r, _ := globalMatch("fmt.Println", "fprint")
	got := markRanges([]Span{{Text: "fmt.", Fg: "#1"}, {Text: "Println", Fg: "#2"}}, r)
	want := []Span{{Text: "f", Fg: "#1", Match: true}, {Text: "mt.", Fg: "#1"}, {Text: "Print", Fg: "#2", Match: true}, {Text: "ln", Fg: "#2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fuzzy markRanges = %+v\nwant %+v", got, want)
	}
}

func TestShiftRanges(t *testing.T) {
	if got := shiftRanges([][2]int{{0, 2}, {1, 4}, {6, 8}}, 2); !reflect.DeepEqual(got, [][2]int{{0, 2}, {4, 6}}) {
		t.Fatalf("shiftRanges = %v", got)
	}
	if got := shiftRanges([][2]int{{1, 2}}, 0); !reflect.DeepEqual(got, [][2]int{{1, 2}}) {
		t.Fatalf("shiftRanges by 0 = %v", got)
	}
}

func TestGlobalSearch(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+/":
			msg = tea.KeyPressMsg{Code: '/', Mod: tea.ModCtrl}
		case "ctrl+_":
			msg = tea.KeyPressMsg{Code: '_', Mod: tea.ModCtrl}
		case "ctrl+k":
			msg = tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		_, cmd := m.handleKey(msg)
		return cmd
	}
	typeText := func(s string) {
		for _, r := range s {
			press(string(r))
		}
	}
	// Both spellings of the key open it; the empty prompt shows a hint.
	press("ctrl+_")
	if m.overlay != overlayGlobal {
		t.Fatal("ctrl+_ should open the all-files search")
	}
	press("esc")
	press("ctrl+/")
	if m.overlay != overlayGlobal || len(m.gsResults) != 0 {
		t.Fatalf("ctrl+/ should open an empty search: overlay=%v", m.overlay)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Search all files") || !strings.Contains(plain, "Type to search every file") {
		t.Fatalf("empty state missing:\n%s", plain)
	}
	// "return" only occurs in b.py: two lines (-return 1, +return 2).
	typeText("return")
	if len(m.gsResults) != 2 || m.gsResults[0].fileIdx != 1 || m.gsResults[0].newNum != 0 || m.gsResults[1].newNum != 2 {
		t.Fatalf("hits: %+v", m.gsResults)
	}
	plain = ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "2 match(es)") || !strings.Contains(plain, "b.py:-2") || !strings.Contains(plain, "b.py:2") || !strings.Contains(plain, "return 2") {
		t.Fatalf("hit rows missing:\n%s", plain)
	}
	// Choose the second hit and jump: file changes, cursor lands on the line,
	// and the query stays as the in-file search.
	press("down")
	press("ctrl+k")
	press("down")
	if m.gsIdx != 1 {
		t.Fatalf("navigation: idx=%d", m.gsIdx)
	}
	press("enter")
	if m.overlay != overlayNone || m.fileIdx != 1 || m.searchQ != "return" {
		t.Fatalf("jump: overlay=%v fileIdx=%d q=%q", m.overlay, m.fileIdx, m.searchQ)
	}
	if _, n, ok := m.rows[m.cursor].nums(); !ok || n != 2 {
		t.Fatalf("cursor should be on new line 2, row=%d", m.cursor)
	}
	// Case-insensitive across files: "PRINTLN" only hits main.go's
	// fmt.Println; the jump back works from another file.
	press("ctrl+/")
	typeText("PRINTLN")
	if len(m.gsResults) != 1 || m.gsResults[0].fileIdx != 0 || !strings.Contains(m.gsResults[0].text, "Println") {
		t.Fatalf("case-insensitive hits: %+v", m.gsResults)
	}
	press("enter")
	if m.fileIdx != 0 || !strings.Contains(m.rows[m.cursor].line.Text, "Println") {
		t.Fatalf("jump: fileIdx=%d cursor=%d", m.fileIdx, m.cursor)
	}
	// Fuzzy: "fprint" only matches fmt.Println as a subsequence. Jumping to
	// a fuzzy hit does not turn it into the (exact) in-file search.
	m.searchQ = "PRINTLN"
	press("ctrl+/")
	typeText("fprint")
	if len(m.gsResults) != 1 || m.gsResults[0].exact || !strings.Contains(m.gsResults[0].text, "Println") {
		t.Fatalf("fuzzy hits: %+v", m.gsResults)
	}
	press("enter")
	if !strings.Contains(m.rows[m.cursor].line.Text, "Println") || m.searchQ != "PRINTLN" {
		t.Fatalf("fuzzy jump: cursor=%d q=%q", m.cursor, m.searchQ)
	}
	// Exact hits are listed before fuzzy ones: "mt" occurs in three lines
	// and is a subsequence of "import (".
	press("ctrl+/")
	typeText("mt")
	if len(m.gsResults) != 4 {
		t.Fatalf("mt hits: %+v", m.gsResults)
	}
	for i, h := range m.gsResults {
		if h.exact != (i < 3) {
			t.Fatalf("exact hits should come first: %+v", m.gsResults)
		}
	}
	if !strings.Contains(m.gsResults[3].text, "import (") {
		t.Fatalf("fuzzy hit last: %+v", m.gsResults[3])
	}
	press("esc")
	// No hits: enter does nothing; backspace refilters; esc cancels.
	press("ctrl+/")
	typeText("zzzz")
	press("enter")
	if m.overlay != overlayGlobal || !strings.Contains(ansi.Strip(m.View().Content), "No matches") {
		t.Fatal("no-match state")
	}
	press("backspace")
	press("backspace")
	press("backspace")
	press("backspace")
	typeText("def")
	if len(m.gsResults) != 1 {
		t.Fatalf("refilter after backspace: %d", len(m.gsResults))
	}
	press("esc")
	if m.overlay != overlayNone || m.fileIdx != 0 {
		t.Fatal("esc cancels without moving")
	}
}
