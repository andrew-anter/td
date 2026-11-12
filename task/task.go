/*
Package Task Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultProject is the namespace used when no --project is given.
// Legacy data files (a bare JSON array of items) migrate into it.
const DefaultProject = "inbox"

type Item struct {
	Text string
	// Description is optional supporting detail; omitzero keeps empty
	// descriptions out of the JSON so old data files round-trip
	// unchanged.
	Description string `json:"Description,omitzero"`
	Priority    int
	Done        bool
	// Created is zero for items written before timestamps existed;
	// omitzero keeps the field out of the JSON for those, so old data
	// files round-trip unchanged. Completed is zero while the item is
	// pending (and for completed items that predate the field).
	Created   time.Time `json:"Created,omitzero"`
	Completed time.Time `json:"Completed,omitzero"`
}

// ByPri implements sort.Interface for []Item: undone first, then by
// ascending priority number (1 is highest, 3 is lowest), then in
// original slice order (stable when used with sort.Stable).
type ByPri []Item

func (s ByPri) Len() int      { return len(s) }
func (s ByPri) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByPri) Less(i, j int) bool {
	if s[i].Done != s[j].Done {
		return s[j].Done
	}
	if s[i].Priority != s[j].Priority {
		return s[i].Priority < s[j].Priority
	}
	return false
}

// SetPriority assigns pri, which must be 1, 2, or 3. Anything else
// leaves the item untouched and returns an error.
func (i *Item) SetPriority(pri int) error {
	switch pri {
	case 1, 2, 3:
		i.Priority = pri
		return nil
	default:
		return fmt.Errorf("invalid priority %d: must be 1, 2, or 3", pri)
	}
}

func (i *Item) PrettyP() string {
	if i.Priority == 1 {
		return "(1)"
	}
	if i.Priority == 3 {
		return "(3)"
	}
	return " "
}

func (i *Item) PrettyDone() string {
	if i.Done {
		return "X"
	}
	return ""
}

// PrettyCreated renders the local calendar date the item was added.
// Items predating timestamps get a placeholder so the column stays
// aligned.
func (i *Item) PrettyCreated() string {
	if i.Created.IsZero() {
		return "-"
	}
	return i.Created.Local().Format("2006-01-02")
}

// PrettyCompleted renders the local date the item was completed, or a
// placeholder while pending.
func (i *Item) PrettyCompleted() string {
	if i.Completed.IsZero() {
		return "-"
	}
	return i.Completed.Local().Format("2006-01-02")
}

// Project is a named namespace for items. Each project keeps its own
// list and its own display numbering: item 3 of "work" has nothing to
// do with item 3 of "home".
type Project struct {
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

// Store is the entire data file: an ordered set of projects.
type Store struct {
	Projects []Project `json:"projects"`
}

// Project returns the named project, or nil when absent.
func (s *Store) Project(name string) *Project {
	for k := range s.Projects {
		if s.Projects[k].Name == name {
			return &s.Projects[k]
		}
	}
	return nil
}

// EnsureProject returns the named project, appending an empty one at
// the end of the store when it does not exist yet.
func (s *Store) EnsureProject(name string) *Project {
	if p := s.Project(name); p != nil {
		return p
	}
	s.Projects = append(s.Projects, Project{Name: name})
	return &s.Projects[len(s.Projects)-1]
}

// RemoveProject deletes the named project and reports whether it
// existed.
func (s *Store) RemoveProject(name string) bool {
	for k := range s.Projects {
		if s.Projects[k].Name == name {
			s.Projects = append(s.Projects[:k], s.Projects[k+1:]...)
			return true
		}
	}
	return false
}

// ProjectNames returns the project names in store order.
func (s *Store) ProjectNames() []string {
	names := make([]string, len(s.Projects))
	for k, p := range s.Projects {
		names[k] = p.Name
	}
	return names
}

// ValidateProjectName rejects blank names and names containing control
// characters. Spaces inside a name are allowed — quote the value on
// the CLI, or just type it in the TUI; callers trim leading/trailing
// whitespace before validating.
func ValidateProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("project name must not be empty")
	}
	for _, r := range name {
		if r < ' ' {
			return fmt.Errorf("project name %q must not contain control characters", name)
		}
	}
	return nil
}

// SaveStore writes the store as JSON, atomically: the payload lands in
// a temp file beside the target which is then renamed over it, so a
// crash mid-write never truncates the real data file.
func SaveStore(filename string, s Store) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writeFileAtomic(filename, b)
}

func writeFileAtomic(filename string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Always close tmp so any later os.Remove can succeed (notably on
	// Windows, where an open handle blocks DeleteFileW).
	defer tmp.Close()

	// After a successful rename, tmpName no longer exists; on any
	// earlier failure, remove the partial file.
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Chmod(0o640); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	renamed = true
	return nil
}

// ReadStore loads the data file. A legacy file — a bare JSON array of
// items, the format that predates projects — migrates transparently:
// its items become the contents of DefaultProject. The upgrade is
// written back on the next save; reading never mutates the file.
func ReadStore(filename string) (Store, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return Store{}, err
	}

	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return Store{}, nil
	}

	if trimmed[0] == '[' {
		var items []Item
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return Store{}, err
		}
		s := Store{}
		if len(items) > 0 {
			s.Projects = []Project{{Name: DefaultProject, Items: items}}
		}
		return s, nil
	}

	var s Store
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return Store{}, err
	}
	return s, nil
}
