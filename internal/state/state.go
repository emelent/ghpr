// Package state persists small per-user review state, such as which files
// of a pull request have been marked as viewed, in a JSON file.
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

// Store is the on-disk state. Keys of Viewed are "owner/repo#123", then the
// file path within the PR.
type Store struct {
	path   string
	Viewed map[string]map[string]Viewed `json:"viewed"`
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
	s := &Store{path: filepath.Join(dir, "viewed.json"), Viewed: map[string]map[string]Viewed{}}
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
	return s, nil
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
