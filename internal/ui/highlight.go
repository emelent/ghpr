package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"ghpr/internal/diff"
)

// Span is a fragment of a line carrying its own foreground styling.
type Span struct {
	Text      string
	Fg        string // hex colour, "" for default
	Bold      bool
	Italic    bool
	Underline bool
}

// Highlighter tokenises source text with chroma and maps tokens to spans.
type Highlighter struct {
	style *chroma.Style
}

// NewHighlighter picks a chroma style by name, falling back to a sane default.
func NewHighlighter(name string) *Highlighter {
	st := styles.Get(name)
	if st == nil || name == "" {
		st = styles.Get("catppuccin-mocha")
	}
	if st == nil {
		st = styles.Fallback
	}
	return &Highlighter{style: st}
}

func expandTabs(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\t", "    ")
}

// lexerFor chooses a lexer by file name, then content analysis.
func (h *Highlighter) lexerFor(path, sample string) chroma.Lexer {
	lexer := lexers.Match(path)
	if lexer == nil && sample != "" {
		lexer = lexers.Analyse(sample)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

// highlightLines tokenises the whole text and returns spans for each line.
func (h *Highlighter) highlightLines(lexer chroma.Lexer, text string) [][]Span {
	n := strings.Count(text, "\n") + 1
	out := make([][]Span, n)
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		// Plain fallback.
		for i, l := range strings.Split(text, "\n") {
			out[i] = []Span{{Text: l}}
		}
		return out
	}
	li := 0
	for _, tok := range it.Tokens() {
		entry := h.style.Get(tok.Type)
		mk := func(s string) Span {
			sp := Span{Text: s}
			if entry.Colour.IsSet() {
				sp.Fg = entry.Colour.String()
			}
			sp.Bold = entry.Bold == chroma.Yes
			sp.Italic = entry.Italic == chroma.Yes
			sp.Underline = entry.Underline == chroma.Yes
			return sp
		}
		parts := strings.Split(tok.Value, "\n")
		for pi, p := range parts {
			if pi > 0 {
				li++
				if li >= n {
					// Should not happen, but never index out of range.
					return out
				}
			}
			if p != "" {
				out[li] = append(out[li], mk(p))
			}
		}
	}
	return out
}

// HighlightFile returns spans for every line of the file. Context lines are
// highlighted as part of the new-side text so multi-line constructs keep
// their state as far as the diff allows.
func (h *Highlighter) HighlightFile(f *diff.File) map[*diff.Line][]Span {
	res := make(map[*diff.Line][]Span)
	var oldLines, newLines []*diff.Line
	var oldSB, newSB strings.Builder
	for hi := range f.Hunks {
		hk := &f.Hunks[hi]
		for li := range hk.Lines {
			l := &hk.Lines[li]
			l.Text = expandTabs(l.Text)
			switch l.Kind {
			case diff.Context:
				oldLines = append(oldLines, l)
				newLines = append(newLines, l)
				oldSB.WriteString(l.Text)
				oldSB.WriteByte('\n')
				newSB.WriteString(l.Text)
				newSB.WriteByte('\n')
			case diff.Del:
				oldLines = append(oldLines, l)
				oldSB.WriteString(l.Text)
				oldSB.WriteByte('\n')
			case diff.Add:
				newLines = append(newLines, l)
				newSB.WriteString(l.Text)
				newSB.WriteByte('\n')
			}
		}
	}
	lexer := h.lexerFor(f.Path(), newSB.String())
	apply := func(lines []*diff.Line, sb *strings.Builder) {
		if len(lines) == 0 {
			return
		}
		text := strings.TrimSuffix(sb.String(), "\n")
		spans := h.highlightLines(lexer, text)
		for i, l := range lines {
			if i < len(spans) {
				res[l] = spans[i]
			} else {
				res[l] = []Span{{Text: l.Text}}
			}
		}
	}
	apply(oldLines, &oldSB)
	apply(newLines, &newSB) // context lines take new-side highlighting
	return res
}

// renderSpans renders spans onto a background, truncating/padding to width.
func renderSpans(spans []Span, bg color.Color, width int) string {
	if width <= 0 {
		return ""
	}
	var sb strings.Builder
	used := 0
	for _, sp := range spans {
		if used >= width {
			break
		}
		txt := sp.Text
		w := lipgloss.Width(txt)
		if used+w > width {
			txt = truncate(txt, width-used)
			w = lipgloss.Width(txt)
		}
		st := lipgloss.NewStyle().Background(bg)
		if sp.Fg != "" {
			st = st.Foreground(lipgloss.Color(sp.Fg))
		}
		if sp.Bold {
			st = st.Bold(true)
		}
		if sp.Italic {
			st = st.Italic(true)
		}
		if sp.Underline {
			st = st.Underline(true)
		}
		sb.WriteString(st.Render(txt))
		used += w
	}
	if used < width {
		sb.WriteString(lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width-used)))
	}
	return sb.String()
}
