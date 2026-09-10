package gh

import "testing"

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
