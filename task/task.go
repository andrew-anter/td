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
	// StateChanged is when Done last changed. GitLab marks bugs and
	// todos closed too, so when the two disagree about completion the
	// more recent change wins.
	StateChanged time.Time `json:"StateChanged,omitzero"`
	// GitLabIssue is the IID of the issue this item mirrors, zero when
	// the item is local only. IIDs are per project, so GitLabProject
	// travels with them: moving an item to another td project must not
	// retarget the number at a different project's issues.
	GitLabIssue   int    `json:"GitLabIssue,omitzero"`
	GitLabProject string `json:"GitLabProject,omitzero"`
}

// MarkDone changes the completion state and records when, so a later
// sync can tell a local reopen from an older remote close. Completed
// mirrors the local clock; adopted remote closes pass the remote's
// closed_at instead.
func (i *Item) MarkDone(done bool, at time.Time) {
	i.Done = done
	i.StateChanged = at
	if done {
		i.Completed = at
	} else {
		i.Completed = time.Time{}
	}
}

// Linked reports whether the item mirrors a GitLab issue.
func (i Item) Linked() bool { return i.GitLabIssue > 0 && i.GitLabProject != "" }

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

// Group is a named collection of projects: the top level of the
// group → project → item hierarchy. Groups hold project names, so a
// project belongs to at most one group; projects absent from every
// group are ungrouped. Empty groups are allowed.
type Group struct {
	Name     string   `json:"name"`
	Projects []string `json:"projects"`
}

// Store is the entire data file: an ordered set of projects plus the
// groups that organize them.
type Store struct {
	// Groups orders the grouped projects for display; omitzero keeps
	// pre-group data files round-tripping unchanged.
	Groups   []Group   `json:"groups,omitzero"`
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
// existed. Group references to it are pruned.
func (s *Store) RemoveProject(name string) bool {
	for k := range s.Projects {
		if s.Projects[k].Name == name {
			s.Projects = append(s.Projects[:k], s.Projects[k+1:]...)
			for g := range s.Groups {
				s.Groups[g].Projects = removeName(s.Groups[g].Projects, name)
			}
			return true
		}
	}
	return false
}

// RenameProject renames a project, keeping its items, and updates
// every group reference to it.
func (s *Store) RenameProject(oldName, newName string) error {
	if err := ValidateProjectName(newName); err != nil {
		return err
	}
	p := s.Project(oldName)
	if p == nil {
		return fmt.Errorf("no such project %q", oldName)
	}
	if s.Project(newName) != nil {
		return fmt.Errorf("project %q already exists", newName)
	}
	p.Name = newName
	for g := range s.Groups {
		for j, n := range s.Groups[g].Projects {
			if n == oldName {
				s.Groups[g].Projects[j] = newName
			}
		}
	}
	return nil
}

// ProjectNames returns the project names in store order.
func (s *Store) ProjectNames() []string {
	names := make([]string, len(s.Projects))
	for k, p := range s.Projects {
		names[k] = p.Name
	}
	return names
}

// Group returns the named group, or nil when absent.
func (s *Store) Group(name string) *Group {
	for k := range s.Groups {
		if s.Groups[k].Name == name {
			return &s.Groups[k]
		}
	}
	return nil
}

// EnsureGroup returns the named group, appending an empty one at the
// end of the store when it does not exist yet.
func (s *Store) EnsureGroup(name string) *Group {
	if g := s.Group(name); g != nil {
		return g
	}
	s.Groups = append(s.Groups, Group{Name: name})
	return &s.Groups[len(s.Groups)-1]
}

// RemoveGroup deletes the named group and reports whether it existed.
// Its projects are kept, becoming ungrouped.
func (s *Store) RemoveGroup(name string) bool {
	for k := range s.Groups {
		if s.Groups[k].Name == name {
			s.Groups = append(s.Groups[:k], s.Groups[k+1:]...)
			return true
		}
	}
	return false
}

// GroupNames returns the group names in store order.
func (s *Store) GroupNames() []string {
	names := make([]string, len(s.Groups))
	for k, g := range s.Groups {
		names[k] = g.Name
	}
	return names
}

// GroupOf returns the name of the group holding the named project, or
// "" when the project is ungrouped or absent.
func (s *Store) GroupOf(project string) string {
	for _, g := range s.Groups {
		for _, n := range g.Projects {
			if n == project {
				return g.Name
			}
		}
	}
	return ""
}

// SetGroup moves the named project into group, creating the group when
// needed; an empty group name ungroups the project. The project must
// exist. A group left empty by the move is kept.
func (s *Store) SetGroup(project, group string) error {
	if s.Project(project) == nil {
		return fmt.Errorf("no such project %q", project)
	}
	for k := range s.Groups {
		s.Groups[k].Projects = removeName(s.Groups[k].Projects, project)
	}
	if group != "" {
		g := s.EnsureGroup(group)
		g.Projects = append(g.Projects, project)
	}
	return nil
}

// GroupedProjectNames returns project names in display order: each
// group's projects in group order, then ungrouped projects in store
// order.
func (s *Store) GroupedProjectNames() []string {
	out := make([]string, 0, len(s.Projects))
	seen := make(map[string]bool, len(s.Projects))
	for _, g := range s.Groups {
		for _, n := range g.Projects {
			if s.Project(n) != nil && !seen[n] {
				out = append(out, n)
				seen[n] = true
			}
		}
	}
	for _, p := range s.Projects {
		if !seen[p.Name] {
			out = append(out, p.Name)
		}
	}
	return out
}

// removeName drops the first occurrence of name from names.
func removeName(names []string, name string) []string {
	for k, n := range names {
		if n == name {
			return append(names[:k], names[k+1:]...)
		}
	}
	return names
}

// ValidateProjectName rejects blank names and names containing control
// characters. Spaces inside a name are allowed — quote the value on
// the CLI, or just type it in the TUI; callers trim leading/trailing
// whitespace before validating.
func ValidateProjectName(name string) error { return validateName(name, "project") }

// ValidateGroupName applies the same rules as ValidateProjectName to
// group names.
func ValidateGroupName(name string) error { return validateName(name, "group") }

func validateName(name, kind string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s name must not be empty", kind)
	}
	for _, r := range name {
		if r < ' ' {
			return fmt.Errorf("%s name %q must not contain control characters", kind, name)
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
	return WriteFileAtomic(filename, b)
}

// WriteFileAtomic writes b to a temp file beside filename and renames
// it over the target, so a crash mid-write cannot leave a truncated
// file behind. The result is mode 0640.
func WriteFileAtomic(filename string, b []byte) error {
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
	s.sanitizeGroups()
	return s, nil
}

// sanitizeGroups enforces the group invariants on loaded data: every
// reference names an existing project, no project sits in two groups,
// and group names are unique. Hand-edited files cannot break display.
func (s *Store) sanitizeGroups() {
	exists := make(map[string]bool, len(s.Projects))
	for _, p := range s.Projects {
		exists[p.Name] = true
	}
	seenGroup := make(map[string]bool, len(s.Groups))
	seenProject := make(map[string]bool)
	groups := s.Groups[:0]
	for _, g := range s.Groups {
		if seenGroup[g.Name] {
			continue
		}
		seenGroup[g.Name] = true
		refs := g.Projects[:0]
		for _, n := range g.Projects {
			if exists[n] && !seenProject[n] {
				seenProject[n] = true
				refs = append(refs, n)
			}
		}
		if len(refs) == 0 {
			refs = nil
		}
		g.Projects = refs
		groups = append(groups, g)
	}
	s.Groups = groups
}
