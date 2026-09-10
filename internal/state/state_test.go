package state

import (
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pr := PRKey("o/r", 7)
	if _, ok := s.Get(pr, "a.go"); ok {
		t.Fatal("empty store should have no entries")
	}
	when := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	s.Set(pr, "a.go", Viewed{ViewedAt: when, HeadSHA: "abc", Fingerprint: "f1"})
	s.Set(pr, "b.go", Viewed{ViewedAt: when, Fingerprint: "f2"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s.SetLast(pr, Position{Path: "b.go", Line: 12, Side: "RIGHT", UpdatedAt: when})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := s2.GetLast(pr); !ok || p.Path != "b.go" || p.Line != 12 || p.Side != "RIGHT" {
		t.Fatalf("last position lost: %+v ok=%v", p, ok)
	}
	if _, ok := s2.GetLast("other#1"); ok {
		t.Fatal("unknown PR has no position")
	}
	v, ok := s2.Get(pr, "a.go")
	if !ok || !v.ViewedAt.Equal(when) || v.HeadSHA != "abc" || v.Fingerprint != "f1" {
		t.Fatalf("round trip lost data: %+v ok=%v", v, ok)
	}
	if got := s2.Paths(pr); len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("paths %v", got)
	}
	s2.Delete(pr, "a.go")
	s2.Delete(pr, "b.go")
	if len(s2.Viewed) != 0 {
		t.Fatal("deleting the last file should drop the PR key")
	}
	if err := s2.Save(); err != nil {
		t.Fatal(err)
	}
	s3, _ := Open(dir)
	if len(s3.Viewed) != 0 {
		t.Fatal("expected empty store after save")
	}
}

func TestDirOverride(t *testing.T) {
	t.Setenv("GHPR_STATE_DIR", "/tmp/ghpr-test-state")
	d, err := Dir()
	if err != nil || d != "/tmp/ghpr-test-state" {
		t.Fatalf("dir %q err %v", d, err)
	}
}
