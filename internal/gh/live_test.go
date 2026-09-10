//go:build live

package gh

import (
	"os"
	"strings"
	"testing"

	"ghpr/internal/diff"
)

// Run with: go test -tags live ./internal/gh -v
// Exercises the read-only gh paths against public pull requests.
func TestLiveReadPaths(t *testing.T) {
	c := &Client{Repo: "charmbracelet/bubbletea"}
	prs, err := c.ListPRs(15)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) == 0 {
		t.Skip("no open PRs")
	}
	t.Logf("listed %d PRs; first #%d %q by %s", len(prs), prs[0].Number, prs[0].Title, prs[0].Author.Login)

	pr, err := c.ViewPR(prs[0].Number)
	if err != nil {
		t.Fatal(err)
	}
	if pr.HeadRefOid == "" || pr.Title == "" {
		t.Fatalf("incomplete PR: %+v", pr)
	}
	t.Logf("view: #%d %s head=%s files=%d +%d -%d", pr.Number, pr.State, pr.HeadRefOid[:8], pr.ChangedFiles, pr.Additions, pr.Deletions)

	text, err := c.Diff(pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	files := diff.Parse(text)
	if len(files) == 0 {
		t.Fatalf("diff parsed to zero files (len=%d)", len(text))
	}
	t.Logf("diff: %d files, first %s (%d hunks)", len(files), files[0].Path(), len(files[0].Hunks))

	// Find a PR with review threads to exercise GraphQL fully.
	found := false
	for _, p := range prs {
		th, err := c.ReviewThreads(p.Number)
		if err != nil {
			t.Fatal(err)
		}
		if len(th) > 0 {
			found = true
			t.Logf("threads: PR #%d has %d threads; first: %s:%d %s resolved=%v comments=%d by @%s",
				p.Number, len(th), th[0].Path, th[0].Line, th[0].DiffSide, th[0].IsResolved, len(th[0].Comments), th[0].Comments[0].Author)
			if th[0].Comments[0].DatabaseID == 0 || th[0].ID == "" {
				t.Fatalf("thread ids missing: %+v", th[0])
			}
			break
		}
	}
	if !found {
		t.Log("no PR with review threads among the first 15; GraphQL query still returned OK")
	}
}

// TestLiveThreads runs the review-thread query against a PR known to have
// threads. Override with GHPR_LIVE_PR="owner/repo#123".
func TestLiveThreads(t *testing.T) {
	ref := os.Getenv("GHPR_LIVE_PR")
	if ref == "" {
		ref = "charmbracelet/bubbletea#1766"
	}
	repo, n, err := ParsePRRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Repo: repo}
	th, err := c.ReviewThreads(n)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d threads", ref, len(th))
	for i, x := range th {
		if i >= 5 {
			break
		}
		t.Logf("  %s:%d side=%s line=%d orig=%d resolved=%v outdated=%v comments=%d id=%s dbid=%d @%s %q",
			x.Path, x.Line, x.DiffSide, x.Line, x.OriginalLine, x.IsResolved, x.IsOutdated, len(x.Comments), x.ID[:12], x.Comments[0].DatabaseID, x.Comments[0].Author, firstLine(x.Comments[0].Body))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}
