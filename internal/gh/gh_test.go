package gh

import (
	"encoding/json"
	"strings"
	"testing"

	"ghpr/internal/diff"
)

func TestLineCommentIsRange(t *testing.T) {
	if (LineComment{Line: 5, Side: "RIGHT"}).IsRange() {
		t.Fatal("single line is not a range")
	}
	if (LineComment{Line: 5, Side: "RIGHT", StartLine: 5, StartSide: "RIGHT"}).IsRange() {
		t.Fatal("same start and end is not a range")
	}
	if !(LineComment{Line: 5, Side: "RIGHT", StartLine: 3, StartSide: "RIGHT"}).IsRange() {
		t.Fatal("3-5 is a range")
	}
	if !(LineComment{Line: 3, Side: "RIGHT", StartLine: 3, StartSide: "LEFT"}).IsRange() {
		t.Fatal("LEFT 3 -> RIGHT 3 is a range")
	}
}

func TestParsePRRef(t *testing.T) {
	cases := []struct {
		in   string
		repo string
		n    int
		ok   bool
	}{
		{"123", "", 123, true},
		{"#45", "", 45, true},
		{"https://github.com/owner/repo/pull/9", "owner/repo", 9, true},
		{"https://github.com/owner/repo/pull/9/files", "owner/repo", 9, true},
		{"owner/repo#77", "owner/repo", 77, true},
		{"nope", "", 0, false},
	}
	for _, tc := range cases {
		repo, n, err := ParsePRRef(tc.in)
		if (err == nil) != tc.ok || repo != tc.repo || n != tc.n {
			t.Errorf("%q: got (%q,%d,%v) want (%q,%d,ok=%v)", tc.in, repo, n, err, tc.repo, tc.n, tc.ok)
		}
	}
}

func TestFilesFromAPI(t *testing.T) {
	entries := []apiFile{
		{Filename: "a.go", Status: "modified", Additions: 1, Deletions: 1, Changes: 2, Patch: "@@ -1,2 +1,2 @@\n-old\n+new\n ctx"},
		{Filename: "new.txt", Status: "added", Additions: 2, Changes: 2, Patch: "@@ -0,0 +1,2 @@\n+hello\n+world"},
		{Filename: "gone.txt", Status: "removed", Deletions: 1, Changes: 1, Patch: "@@ -1 +0,0 @@\n-bye"},
		{Filename: "b/new.go", PreviousFilename: "a/old.go", Status: "renamed"},
		{Filename: "img.png", Status: "added"},
		{Filename: "huge.json", Status: "modified", Additions: 5000, Deletions: 4000, Changes: 9000},
	}
	files := filesFromAPI(entries)
	if len(files) != 6 {
		t.Fatalf("%d files", len(files))
	}
	a := files[0]
	if a.Status != diff.Modified || a.Path() != "a.go" || len(a.Hunks) != 1 || len(a.Hunks[0].Lines) != 3 || a.Additions != 1 || a.Deletions != 1 {
		t.Fatalf("modified: %+v", a)
	}
	if n := files[1]; n.Status != diff.Added || n.OldPath != "" || n.Path() != "new.txt" || len(n.Hunks) != 1 || n.Hunks[0].Lines[0].Text != "hello" {
		t.Fatalf("added: %+v", n)
	}
	if g := files[2]; g.Status != diff.Deleted || g.NewPath != "" || g.Path() != "gone.txt" || g.Hunks[0].Lines[0].Kind != diff.Del {
		t.Fatalf("removed: %+v", g)
	}
	if r := files[3]; r.Status != diff.Renamed || r.OldPath != "a/old.go" || r.NewPath != "b/new.go" || r.IsBinary || r.PatchOmitted || len(r.Hunks) != 0 {
		t.Fatalf("renamed: %+v", r)
	}
	if b := files[4]; !b.IsBinary || b.PatchOmitted {
		t.Fatalf("binary: %+v", b)
	}
	if h := files[5]; !h.PatchOmitted || h.IsBinary || len(h.Hunks) != 0 || h.Additions != 5000 {
		t.Fatalf("omitted patch: %+v", h)
	}
}

func TestIsTooManyFiles(t *testing.T) {
	err := &Error{Args: []string{"pr", "diff", "1"}, Stderr: "could not find pull request diff: HTTP 406: Sorry, the diff exceeded the maximum number of files (300). Consider using 'List pull requests files' API or locally cloning the repository instead"}
	if !isTooManyFiles(err) {
		t.Fatal("406 should trigger the files API fallback")
	}
	if isTooManyFiles(&Error{Args: []string{"pr", "diff", "1"}, Stderr: "HTTP 404: Not Found"}) {
		t.Fatal("other errors must not")
	}
}

func TestViewedFilesResponseDecode(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"files":{"pageInfo":{"hasNextPage":true,"endCursor":"c2"},"nodes":[{"path":"a.go","viewerViewedState":"VIEWED"},{"path":"b.go","viewerViewedState":"UNVIEWED"},{"path":"c.go","viewerViewedState":"DISMISSED"}]}}}}}`
	var resp viewedFilesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	files := resp.Data.Repository.PullRequest.Files
	if !files.PageInfo.HasNextPage || files.PageInfo.EndCursor != "c2" || len(files.Nodes) != 3 || files.Nodes[0].State != "VIEWED" || files.Nodes[2].State != "DISMISSED" {
		t.Fatalf("decoded %+v", files)
	}
	if err := graphqlErrors([]byte(`{"errors":[{"message":"bad"}]}`)); err == nil || err.Error() != "graphql: bad" {
		t.Fatalf("graphqlErrors = %v", err)
	}
	if err := graphqlErrors([]byte(`{"data":{}}`)); err != nil {
		t.Fatalf("clean response: %v", err)
	}
}

func TestApprovers(t *testing.T) {
	mk := func(login, state string) ReviewState {
		var r ReviewState
		r.Author.Login, r.State = login, state
		return r
	}
	approved, changes := Approvers([]ReviewState{mk("zoe", "APPROVED"), mk("bob", "COMMENTED"), mk("carol", "CHANGES_REQUESTED"), mk("alice", "APPROVED"), mk("dan", "DISMISSED")})
	if strings.Join(approved, ",") != "alice,zoe" || strings.Join(changes, ",") != "carol" {
		t.Fatalf("approved=%v changes=%v", approved, changes)
	}
	if a, c := Approvers(nil); a != nil || c != nil {
		t.Fatal("no reviews")
	}
}
