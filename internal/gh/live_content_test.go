//go:build live

package gh

import (
	"strings"
	"testing"

	"ghpr/internal/diff"
)

// Run with: go test -tags live ./internal/gh -run TestLiveFileContent -v
func TestLiveFileContent(t *testing.T) {
	c := &Client{Repo: "charmbracelet/bubbletea"}
	prs, err := c.ListPRs(5)
	if err != nil || len(prs) == 0 {
		t.Skip("no open PRs", err)
	}
	pr, err := c.ViewPR(prs[0].Number)
	if err != nil {
		t.Fatal(err)
	}
	text, err := c.Diff(pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	files := diff.Parse(text)
	for i := range files {
		f := &files[i]
		if f.IsBinary || f.Status == diff.Deleted {
			continue
		}
		content, err := c.FileContent(pr.HeadRefOid, f.Path())
		if err != nil {
			t.Fatal(err)
		}
		full := diff.Expand(f, content)
		n := len(strings.Split(strings.TrimSuffix(content, "\n"), "\n"))
		t.Logf("%s: %d lines in file, %d rows expanded", f.Path(), n, len(full.Hunks[0].Lines))
		// Every context line from the hunks must match the fetched file.
		lines := strings.Split(content, "\n")
		for _, l := range full.Hunks[0].Lines {
			if l.Kind == diff.Del || l.NewNum == 0 || l.NewNum > len(lines) {
				continue
			}
			if strings.ReplaceAll(lines[l.NewNum-1], "\t", "    ") != strings.ReplaceAll(l.Text, "\t", "    ") {
				t.Fatalf("%s:%d mismatch: file %q vs diff %q", f.Path(), l.NewNum, lines[l.NewNum-1], l.Text)
			}
		}
		return
	}
	t.Skip("no eligible file")
}
