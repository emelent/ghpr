// Package state persists small per-user review state, such as which files
// of a pull request have been marked as viewed and the reviewer's notes on
// lines, in a JSON file.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Viewed records that a file was marked as viewed.
type Viewed struct {
	ViewedAt time.Time `json:"viewedAt"`
	HeadSHA  string    `json:"headSha,omitempty"`
	// Fingerprint identifies the file's diff at the time it was viewed. When
	// the current diff no longer matches, the file changed after it was
	// viewed and the mark is dropped.
	Fingerprint string `json:"fingerprint"`
}

// Position remembers where the user last was in a pull request.
type Position struct {
	Path      string    `json:"path"`
	Line      int       `json:"line,omitempty"` // new-side line (or old-side when 0 on the new side)
	Side      string    `json:"side,omitempty"` // RIGHT or LEFT
	UpdatedAt time.Time `json:"updatedAt"`
}

// Note is a reviewer's note on a diff line, something to come back to.
type Note struct {
	Path      string    `json:"path"`
	OldLine   int       `json:"oldLine,omitempty"` // old-side line; the note is on a removed line when NewLine is 0
	NewLine   int       `json:"newLine,omitempty"` // new-side line
	Kind      string    `json:"kind,omitempty"`    // context, add or del
	Text      string    `json:"text,omitempty"`    // code on the line when the note was made
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store is the on-disk state. Keys of Viewed, Last and Notes are
// "owner/repo#123"; Viewed is further keyed by file path within the PR.
type Store struct {
	path   string
	Viewed map[string]map[string]Viewed `json:"viewed"`
	Last   map[string]Position          `json:"last,omitempty"`
	Notes  map[string][]Note            `json:"notes,omitempty"`
}

// Dir returns the directory used for state: $GHPR_STATE_DIR, otherwise
// <user config dir>/ghpr.
func Dir() (string, error) {
	if d := os.Getenv("GHPR_STATE_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghpr"), nil
}

// Open loads the store from dir (creating an empty one when the file does
// not exist yet).
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "viewed.json"), Viewed: map[string]map[string]Viewed{}, Last: map[string]Position{}, Notes: map[string][]Note{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("decode %s: %w", s.path, err)
	}
	if s.Viewed == nil {
		s.Viewed = map[string]map[string]Viewed{}
	}
	if s.Last == nil {
		s.Last = map[string]Position{}
	}
	if s.Notes == nil {
		s.Notes = map[string][]Note{}
	}
	return s, nil
}

// GetNotes returns the notes recorded for a PR, in creation order.
func (s *Store) GetNotes(pr string) []Note {
	return append([]Note(nil), s.Notes[pr]...)
}

// SetNotes replaces the notes for a PR; an empty list removes the entry.
func (s *Store) SetNotes(pr string, notes []Note) {
	if len(notes) == 0 {
		delete(s.Notes, pr)
		return
	}
	s.Notes[pr] = append([]Note(nil), notes...)
}

// Path returns the backing file.
func (s *Store) Path() string { return s.path }

// PRKey builds the key for a pull request.
func PRKey(repo string, number int) string { return fmt.Sprintf("%s#%d", repo, number) }

// Get returns the viewed record for a file, if any.
func (s *Store) Get(pr, path string) (Viewed, bool) {
	v, ok := s.Viewed[pr][path]
	return v, ok
}

// Set marks a file as viewed.
func (s *Store) Set(pr, path string, v Viewed) {
	if s.Viewed[pr] == nil {
		s.Viewed[pr] = map[string]Viewed{}
	}
	s.Viewed[pr][path] = v
}

// Delete removes the viewed mark for a file.
func (s *Store) Delete(pr, path string) {
	if m := s.Viewed[pr]; m != nil {
		delete(m, path)
		if len(m) == 0 {
			delete(s.Viewed, pr)
		}
	}
}

// GetLast returns the last position recorded for a PR.
func (s *Store) GetLast(pr string) (Position, bool) {
	p, ok := s.Last[pr]
	return p, ok
}

// SetLast records the last position for a PR.
func (s *Store) SetLast(pr string, p Position) {
	if s.Last == nil {
		s.Last = map[string]Position{}
	}
	s.Last[pr] = p
}

// Paths lists the viewed files of a PR, sorted.
func (s *Store) Paths(pr string) []string {
	var out []string
	for p := range s.Viewed[pr] {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Save writes the store atomically.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
