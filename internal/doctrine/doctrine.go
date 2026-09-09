// Package doctrine serves the shared curriculum units. The units are
// markdown files on disk, COPY'd into the image at build time, never
// go:embed. Every read goes to disk, so a changed file shows up
// without a restart. Role content never lives here; it comes live
// from Soil.
package doctrine

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrNotFound reports that no unit with the requested slug exists.
var ErrNotFound = errors.New("doctrine: unit not found")

// Unit is one curriculum unit.
type Unit struct {
	// Slug identifies the unit in URLs, the file name without the
	// order prefix and the .md suffix.
	Slug string

	// Title is the first level-one heading of the file, or the slug
	// when the file has none.
	Title string

	// Summary is the first paragraph after the title.
	Summary string

	// Content is the full markdown source.
	Content string
}

// Store reads doctrine units from <dir>/doctrine/*.md. File names
// order the units: NN-slug.md sorts by NN.
type Store struct {
	dir string
}

// NewStore builds a store over the content directory.
func NewStore(contentDir string) *Store {
	return &Store{dir: filepath.Join(contentDir, "doctrine")}
}

// orderPrefix strips the "NN-" ordering prefix from a file name.
var orderPrefix = regexp.MustCompile(`^\d+-`)

// Units lists all units in file order. A missing or empty directory
// returns an empty list and no error: the portal then says the
// curriculum is not installed instead of failing.
func (s *Store) Units() ([]Unit, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	units := make([]Unit, 0, len(names))
	for _, name := range names {
		unit, err := s.load(name)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

// Unit returns one unit by slug. The slug is matched against the
// directory listing, never joined into a path, so a crafted slug
// cannot escape the content directory.
func (s *Store) Unit(slug string) (Unit, error) {
	units, err := s.Units()
	if err != nil {
		return Unit{}, err
	}
	for _, unit := range units {
		if unit.Slug == slug {
			return unit, nil
		}
	}
	return Unit{}, ErrNotFound
}

func (s *Store) load(fileName string) (Unit, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, fileName))
	if err != nil {
		return Unit{}, err
	}
	slug := orderPrefix.ReplaceAllString(strings.TrimSuffix(fileName, ".md"), "")
	content := string(raw)

	unit := Unit{Slug: slug, Title: slug, Content: content}
	lines := strings.Split(content, "\n")
	titleAt := -1
	for i, line := range lines {
		if title, ok := strings.CutPrefix(line, "# "); ok {
			unit.Title = strings.TrimSpace(title)
			titleAt = i
			break
		}
	}
	for _, line := range lines[titleAt+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if unit.Summary != "" {
				break
			}
			continue
		}
		if unit.Summary != "" {
			unit.Summary += " "
		}
		unit.Summary += trimmed
	}
	return unit, nil
}
