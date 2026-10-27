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

func TestNotesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pr := PRKey("o/r", 7)
	if got := s.GetNotes(pr); len(got) != 0 {
		t.Fatalf("fresh store has notes: %v", got)
	}
	notes := []Note{
		{Path: "a.go", NewLine: 4, Kind: "add", Text: `"fmt"`, Body: "check import order", CreatedAt: time.Now().Truncate(time.Second)},
		{Path: "a.go", OldLine: 3, Kind: "del", Text: `import "fmt"`, Body: "why removed?", CreatedAt: time.Now().Truncate(time.Second)},
	}
	s.SetNotes(pr, notes)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.GetNotes(pr)
	if len(got) != 2 || got[0].Body != "check import order" || got[0].NewLine != 4 || got[1].OldLine != 3 || got[1].Kind != "del" || !got[1].CreatedAt.Equal(notes[1].CreatedAt) {
		t.Fatalf("round trip: %+v", got)
	}
	// Mutating the returned slice does not touch the store.
	got[0].Body = "changed"
	if s2.GetNotes(pr)[0].Body != "check import order" {
		t.Fatal("GetNotes should return a copy")
	}
	// Emptying removes the PR entry entirely.
	s2.SetNotes(pr, nil)
	if _, ok := s2.Notes[pr]; ok {
		t.Fatal("empty notes should drop the key")
	}
	// Viewed marks and last position are unaffected by notes.
	s2.Set(pr, "a.go", Viewed{ViewedAt: time.Now(), Fingerprint: "x"})
	if err := s2.Save(); err != nil {
		t.Fatal(err)
	}
	s3, _ := Open(dir)
	if _, ok := s3.Get(pr, "a.go"); !ok || len(s3.GetNotes(pr)) != 0 {
		t.Fatal("viewed marks and notes are independent")
	}
}

func TestChatsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pr := PRKey("o/r", 7)
	at := time.Now().Truncate(time.Second)
	s.SetChats(pr, []Chat{{
		ID: "1", Path: "a.go", Loc: "a.go:3-4", Line: 4, Side: "RIGHT",
		Comment: ChatComment{Line: 4, Side: "RIGHT", StartLine: 3, StartSide: "RIGHT"},
		Dir:     "/src/repo", Session: "s1", Context: "ctx", Fallback: "fb",
		Entries:   []ChatEntry{{Q: "why?", A: "because", At: at}},
		CreatedAt: at, UpdatedAt: at,
	}})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.GetChats(pr)
	if len(got) != 1 || got[0].Session != "s1" || got[0].Comment.StartLine != 3 || got[0].Entries[0].A != "because" || !got[0].Entries[0].At.Equal(at) {
		t.Fatalf("round trip: %+v", got)
	}
	got[0].Loc = "changed"
	if s2.GetChats(pr)[0].Loc != "a.go:3-4" {
		t.Fatal("GetChats should return a copy")
	}
	s2.SetChats(pr, nil)
	if _, ok := s2.Chats[pr]; ok {
		t.Fatal("empty chats should drop the key")
	}
}
