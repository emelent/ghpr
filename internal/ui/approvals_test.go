package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/gh"
)

func TestApprovalsShown(t *testing.T) {
	m := newTestModel(t)
	var a, c gh.ReviewState
	a.Author.Login, a.State = "alice", "APPROVED"
	c.Author.Login, c.State = "carol", "CHANGES_REQUESTED"
	plain := ansi.Strip(m.View().Content)
	if strings.Contains(plain, "✓ alice") {
		t.Fatal("no reviews yet")
	}
	m.pr.ReviewDecision = "APPROVED"
	m.pr.LatestReviews = []gh.ReviewState{a}
	plain = ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "APPROVED ✓ alice") {
		t.Fatalf("header should name the approver:\n%s", strings.SplitN(plain, "\n", 3)[1])
	}
	m.pr.LatestReviews = []gh.ReviewState{a, c}
	plain = ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "✓ alice ✗ carol") {
		t.Fatalf("header should show both:\n%s", strings.SplitN(plain, "\n", 3)[1])
	}
	// The PR list description carries the same summary.
	s := gh.PRSummary{Number: 3, Title: "x", ReviewDecision: "APPROVED", HeadRefName: "b", LatestReviews: []gh.ReviewState{a, c}}
	s.Author.Login = "dev"
	if d := (prItem{s}).Description(); !strings.Contains(d, "APPROVED ✓ alice ✗ carol") {
		t.Fatalf("list description = %q", d)
	}
	s.LatestReviews = nil
	if d := (prItem{s}).Description(); strings.Contains(d, "✓") || strings.Contains(d, "✗") {
		t.Fatalf("no reviews: %q", d)
	}
}
