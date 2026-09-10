package gh

import "testing"

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
