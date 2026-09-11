package ui

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFindMatches(t *testing.T) {
	cases := []struct {
		text, q string
		want    [][2]int
	}{
		{`import "fmt"`, "fmt", [][2]int{{8, 11}}},
		{"Fmt fmt FMT", "fmt", [][2]int{{0, 3}, {4, 7}, {8, 11}}},  // smart case: all lower folds
		{"Fmt fmt FMT", "Fmt", [][2]int{{0, 3}}},                   // capitals: exact
		{"aaaa", "aa", [][2]int{{0, 2}, {2, 4}}},                   // non-overlapping
		{"héllo wörld héllo", "héllo", [][2]int{{0, 5}, {12, 17}}}, // rune offsets
		{"abc", "", nil},
		{"abc", "abcd", nil},
	}
	for _, c := range cases {
		if got := findMatches(c.text, c.q); !reflect.DeepEqual(got, c.want) {
			t.Errorf("findMatches(%q, %q) = %v, want %v", c.text, c.q, got, c.want)
		}
	}
}

func TestMarkSpans(t *testing.T) {
	spans := []Span{{Text: "fmt", Fg: "#1"}, {Text: ".Println(", Fg: "#2"}, {Text: `"hi fmt"`, Fg: "#3"}}
	got := markSpans(spans, "fmt")
	want := []Span{
		{Text: "fmt", Fg: "#1", Match: true},
		{Text: ".Println(", Fg: "#2"},
		{Text: `"hi `, Fg: "#3"},
		{Text: "fmt", Fg: "#3", Match: true},
		{Text: `"`, Fg: "#3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("markSpans = %+v\nwant %+v", got, want)
	}
	// A hit straddling two spans is split across both, keeping each span's style.
	got = markSpans([]Span{{Text: "ab", Fg: "#1"}, {Text: "cd", Fg: "#2"}}, "bc")
	want = []Span{{Text: "a", Fg: "#1"}, {Text: "b", Fg: "#1", Match: true}, {Text: "c", Fg: "#2", Match: true}, {Text: "d", Fg: "#2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("straddling markSpans = %+v\nwant %+v", got, want)
	}
	// No hit: spans are returned untouched.
	if got := markSpans(spans, "zzz"); !reflect.DeepEqual(got, spans) {
		t.Fatalf("no-hit markSpans changed spans: %+v", got)
	}
}

func TestSearch(t *testing.T) {
	m := newTestModel(t)
	press := func(k string) tea.Cmd {
		var msg tea.KeyPressMsg
		switch k {
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
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
	// main.go inline rows: 3 = -import "fmt", 6 = +"fmt", 11 = fmt.Println.
	m.cursor = 0
	press("/")
	if m.overlay != overlaySearch {
		t.Fatal("/ should open the search prompt")
	}
	typeText("fmt")
	if m.cursor != 3 || m.searchQ != "fmt" {
		t.Fatalf("typing should preview the first match: cursor=%d q=%q", m.cursor, m.searchQ)
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "/fmt") || !strings.Contains(plain, "3 match(es)") {
		t.Fatalf("prompt and count missing:\n%s", plain)
	}
	press("backspace")
	if m.searchQ != "fm" {
		t.Fatalf("backspace: q=%q", m.searchQ)
	}
	press("t")
	press("enter")
	if m.overlay != overlayNone || m.cursor != 3 || m.searchQ != "fmt" {
		t.Fatalf("enter keeps the match: overlay=%v cursor=%d q=%q", m.overlay, m.cursor, m.searchQ)
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "1/3") {
		t.Fatalf("active search should show position:\n%s", plain)
	}
	// n / N step through matches, wrapping.
	press("n")
	press("n")
	if m.cursor != 11 {
		t.Fatalf("n twice: cursor=%d", m.cursor)
	}
	press("n")
	if m.cursor != 3 {
		t.Fatalf("n wraps: cursor=%d", m.cursor)
	}
	press("N")
	if m.cursor != 11 {
		t.Fatalf("N wraps back: cursor=%d", m.cursor)
	}
	// Matches are highlighted in the rendered diff.
	if !strings.Contains(m.View().Content, lipglossBg(colMatchBg)) {
		t.Fatal("match highlight colour missing from the view")
	}
	// esc clears the search; n goes back to jumping threads.
	press("esc")
	if m.searchQ != "" {
		t.Fatal("esc should clear the search")
	}
	m.cursor = 3
	press("n")
	if m.cursor != 4 || m.rows[4].kind != rowThread {
		t.Fatalf("n without a search jumps to the next thread: cursor=%d", m.cursor)
	}
	// esc in the prompt restores the cursor and the previous query.
	press("/")
	typeText("fmt")
	press("enter")
	m.cursor = 6
	press("/")
	typeText("package")
	if m.cursor != 1 {
		t.Fatalf("preview should wrap to row 1: cursor=%d", m.cursor)
	}
	press("esc")
	if m.cursor != 6 || m.searchQ != "fmt" || m.overlay != overlayNone {
		t.Fatalf("esc restores: cursor=%d q=%q", m.cursor, m.searchQ)
	}
	// Smart case: capitals make the search exact.
	press("/")
	typeText("FMT")
	press("enter")
	if !m.statusErr || !strings.Contains(m.status, "No match for “FMT”") {
		t.Fatalf("expected no-match status, got %q", m.status)
	}
	// Selection takes esc first; the search survives it.
	press("/")
	typeText("fmt")
	press("enter")
	press("V")
	press("esc")
	if m.selecting || m.searchQ != "fmt" {
		t.Fatal("esc should cancel the selection before clearing the search")
	}
	// Leaving for the PR list drops the search.
	m.backToList()
	if m.searchQ != "" {
		t.Fatal("backToList should clear the search")
	}
}

// lipglossBg returns the SGR background sequence lipgloss emits for c, so a
// test can check that a colour appears in rendered output.
func lipglossBg(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return "48;2;" + strconv.Itoa(int(r>>8)) + ";" + strconv.Itoa(int(g>>8)) + ";" + strconv.Itoa(int(b>>8))
}
