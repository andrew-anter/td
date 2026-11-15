/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSetPriority(t *testing.T) {
	cases := []struct {
		in, want int
		wantOk   bool
	}{
		{1, 1, true},
		{2, 2, true},
		{3, 3, true},
		// Invalid values must error and leave the item untouched.
		{0, 0, false},
		{4, 0, false},
		{-1, 0, false},
	}
	for _, c := range cases {
		var it Item
		err := it.SetPriority(c.in)
		if c.wantOk != (err == nil) {
			t.Errorf("SetPriority(%d) error = %v, wantOk = %v", c.in, err, c.wantOk)
		}
		if it.Priority != c.want {
			t.Errorf("SetPriority(%d) left Priority = %d, want %d", c.in, it.Priority, c.want)
		}
	}
}

func TestByPriLess(t *testing.T) {
	mk := func(pri int, done bool) Item {
		return Item{Text: "x", Priority: pri, Done: done}
	}

	// Done items sort after not-done, regardless of priority.
	undoneHigh := mk(1, false)
	doneLow := mk(3, true)
	items := ByPri{undoneHigh, doneLow}
	if !items.Less(0, 1) {
		t.Errorf("undone item should sort before done item")
	}

	// Lower numeric priority (= higher importance) sorts first when both undone.
	a := mk(1, false)
	b := mk(2, false)
	items = ByPri{a, b}
	if !items.Less(0, 1) {
		t.Errorf("priority 1 should sort before priority 2")
	}

	// Equal priority + done: not Less in either direction (stable).
	x := mk(2, false)
	y := mk(2, false)
	items = ByPri{x, y}
	if items.Less(0, 1) || items.Less(1, 0) {
		t.Errorf("equal items should compare as not less in either direction")
	}
}

func TestByPriStableSort(t *testing.T) {
	items := ByPri{
		{Text: "a", Priority: 2},
		{Text: "b", Priority: 1},
		{Text: "c", Priority: 2},
		{Text: "d", Priority: 1},
	}
	sort.Stable(items)
	got := []string{items[0].Text, items[1].Text, items[2].Text, items[3].Text}
	// Priority 1 (high) first, priority 2 next; within a tier, original order preserved.
	want := []string{"b", "d", "a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sort order = %v, want %v", got, want)
	}
}

func TestSaveAndReadStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	in := Store{Projects: []Project{
		{Name: "inbox", Items: []Item{
			{Text: "first", Priority: 1, Done: false},
			{Text: "second", Priority: 3, Done: true},
		}},
		{Name: "work", Items: []Item{{Text: "ship v2", Priority: 2, Done: false}}},
		{Name: "empty"},
	}}

	if err := SaveStore(path, in); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}

	out, err := ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

// A data file in the pre-projects format (a bare JSON array of items)
// must load with its items in the default project, and reading must
// not rewrite the file.
func TestReadStoreMigratesLegacyArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	legacy := `[{"Text":"milk","Priority":2,"Done":false},{"Text":"paid","Priority":1,"Done":true}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	p := s.Project(DefaultProject)
	if p == nil {
		t.Fatalf("legacy items did not land in %q: %+v", DefaultProject, s)
	}
	if len(p.Items) != 2 || p.Items[0].Text != "milk" || !p.Items[1].Done {
		t.Errorf("migrated items = %+v", p.Items)
	}
	if b, _ := os.ReadFile(path); string(b) != legacy {
		t.Errorf("read mutated the data file")
	}
}

func TestReadStoreEmptyVariants(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{"[]", "", "   "} {
		path := filepath.Join(dir, "tasks.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := ReadStore(path)
		if err != nil {
			t.Fatalf("ReadStore(%q): %v", content, err)
		}
		if len(s.Projects) != 0 {
			t.Errorf("ReadStore(%q) = %+v, want no projects", content, s)
		}
	}
}

func TestSaveStoreRejectsBadPath(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "does", "not", "exist", "tasks.json")
	if err := SaveStore(bad, Store{Projects: []Project{{Name: "x"}}}); err == nil {
		t.Fatalf("expected error saving into non-existent directory")
	}
	if _, err := os.Stat(bad); err == nil {
		t.Errorf("partial file was created at %s", bad)
	}
}

func TestStoreProjectMethods(t *testing.T) {
	s := Store{}
	if s.Project("work") != nil {
		t.Errorf("Project on empty store should be nil")
	}
	p := s.EnsureProject("work")
	p.Items = append(p.Items, Item{Text: "a", Priority: 2})
	if got := s.Project("work"); got == nil || len(got.Items) != 1 {
		t.Fatalf("EnsureProject append not visible through Project: %+v", s)
	}
	s.EnsureProject("work") // idempotent
	if len(s.Projects) != 1 {
		t.Errorf("EnsureProject duplicated the project: %+v", s.Projects)
	}
	s.EnsureProject("home")
	if names := s.ProjectNames(); !reflect.DeepEqual(names, []string{"work", "home"}) {
		t.Errorf("ProjectNames = %v", names)
	}
	if !s.RemoveProject("work") || s.RemoveProject("work") {
		t.Errorf("RemoveProject should report true once, then false")
	}
	if names := s.ProjectNames(); !reflect.DeepEqual(names, []string{"home"}) {
		t.Errorf("after remove ProjectNames = %v", names)
	}
}

func TestValidateProjectName(t *testing.T) {
	ok := []string{"work", "home-chores", "v2", "a1", "two words", "home chores"}
	bad := []string{"", " ", "\t", "tab\there", "a\nb"}
	for _, n := range ok {
		if err := ValidateProjectName(n); err != nil {
			t.Errorf("ValidateProjectName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateProjectName(n); err == nil {
			t.Errorf("ValidateProjectName(%q) = nil, want error", n)
		}
	}
}

// Items that predate timestamps must not gain a bogus Created field
// when the store is rewritten, while new items keep theirs.
func TestCreatedOmittedForLegacyItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	when := time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	done := time.Date(2026, 11, 12, 17, 0, 0, 0, time.UTC)
	in := Store{Projects: []Project{{Name: DefaultProject, Items: []Item{
		{Text: "legacy", Priority: 2, Done: true},
		{Text: "stamped", Priority: 2, Created: when, Completed: done},
	}}}}
	if err := SaveStore(path, in); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Projects []struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items := decoded.Projects[0].Items
	for _, key := range []string{"Created", "Completed"} {
		if _, ok := items[0][key]; ok {
			t.Errorf("legacy item gained a %s field: %s", key, items[0][key])
		}
		if _, ok := items[1][key]; !ok {
			t.Errorf("stamped item lost its %s field: %s", key, raw)
		}
	}

	out, err := ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if got := out.Projects[0].Items[1].Created; !got.Equal(when) {
		t.Errorf("Created round-trip = %v, want %v", got, when)
	}
	if got := out.Projects[0].Items[1].Completed; !got.Equal(done) {
		t.Errorf("Completed round-trip = %v, want %v", got, done)
	}
	if !out.Projects[0].Items[0].Completed.IsZero() {
		t.Errorf("legacy Completed = %v, want zero", out.Projects[0].Items[0].Completed)
	}
}

func TestDescriptionOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	in := Store{Projects: []Project{{Name: DefaultProject, Items: []Item{{Text: "milk", Priority: 2}}}}}
	if err := SaveStore(path, in); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Description") {
		t.Errorf("empty Description leaked into the JSON: %s", raw)
	}

	in.Projects[0].Items[0].Description = "the 2% kind"
	if err := SaveStore(path, in); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	got, err := ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if got.Projects[0].Items[0].Description != "the 2% kind" {
		t.Errorf("Description did not round-trip: %+v", got.Projects[0].Items[0])
	}
}

func TestStoreGroupMethods(t *testing.T) {
	s := Store{Projects: []Project{{Name: "inbox"}, {Name: "work"}, {Name: "home"}}}
	if err := s.SetGroup("work", "A"); err != nil {
		t.Fatalf("SetGroup: %v", err)
	}
	if err := s.SetGroup("home", "A"); err != nil {
		t.Fatalf("SetGroup: %v", err)
	}
	if got := s.GroupOf("work"); got != "A" {
		t.Errorf("GroupOf(work) = %q, want A", got)
	}
	if g := s.Group("A"); g == nil || !reflect.DeepEqual(g.Projects, []string{"work", "home"}) {
		t.Errorf("group A = %+v, want work and home", s.Group("A"))
	}
	// Moving to another group leaves the old group empty but intact.
	if err := s.SetGroup("home", "B"); err != nil {
		t.Fatalf("SetGroup move: %v", err)
	}
	if got := s.GroupOf("home"); got != "B" {
		t.Errorf("GroupOf(home) = %q, want B", got)
	}
	if g := s.Group("A"); g == nil || !reflect.DeepEqual(g.Projects, []string{"work"}) {
		t.Errorf("group A after move = %+v", s.Group("A"))
	}
	if err := s.SetGroup("work", ""); err != nil {
		t.Fatalf("SetGroup ungroup: %v", err)
	}
	if got := s.GroupOf("work"); got != "" {
		t.Errorf("GroupOf(work) after ungroup = %q", got)
	}
	if s.Group("A") == nil {
		t.Errorf("empty group should be kept")
	}
	if err := s.SetGroup("ghost", "A"); err == nil {
		t.Errorf("SetGroup on a missing project should error")
	}
	if !s.RemoveGroup("B") || s.RemoveGroup("B") {
		t.Errorf("RemoveGroup should report true once, then false")
	}
	if s.Project("home") == nil || s.GroupOf("home") != "" {
		t.Errorf("RemoveGroup must keep its projects, ungrouped")
	}
}

func TestGroupedProjectNamesOrder(t *testing.T) {
	s := Store{
		Groups: []Group{
			{Name: "A", Projects: []string{"work", "home"}},
			{Name: "B", Projects: []string{"gym"}},
		},
		Projects: []Project{{Name: "inbox"}, {Name: "work"}, {Name: "home"}, {Name: "gym"}},
	}
	want := []string{"work", "home", "gym", "inbox"}
	if got := s.GroupedProjectNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("GroupedProjectNames = %v, want %v", got, want)
	}
}

// Group membership is stored by name, so renaming or deleting a
// project must update every reference to it.
func TestGroupRefsFollowRenameAndDelete(t *testing.T) {
	s := Store{
		Groups:   []Group{{Name: "A", Projects: []string{"work", "home"}}},
		Projects: []Project{{Name: "work"}, {Name: "home"}},
	}
	if err := s.RenameProject("work", "job"); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	if got := s.Group("A").Projects; !reflect.DeepEqual(got, []string{"job", "home"}) {
		t.Errorf("refs after rename = %v", got)
	}
	s.RemoveProject("home")
	if got := s.Group("A").Projects; !reflect.DeepEqual(got, []string{"job"}) {
		t.Errorf("refs after delete = %v", got)
	}
}

// Hand-edited files must not break display: dangling refs, duplicate
// memberships, and duplicate group names are cleaned up on load.
func TestReadStoreSanitizesGroups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	raw := `{"groups":[{"name":"A","projects":["work","ghost","work"]},{"name":"A","projects":["home"]},{"name":"B","projects":["work","home"]}],"projects":[{"name":"work","items":[]},{"name":"home","items":[]}]}`
	if err := os.WriteFile(path, []byte(raw), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	want := []Group{{Name: "A", Projects: []string{"work"}}, {Name: "B", Projects: []string{"home"}}}
	if !reflect.DeepEqual(s.Groups, want) {
		t.Errorf("sanitized groups = %+v, want %+v", s.Groups, want)
	}
}

// Pre-group data files must round-trip without gaining a groups key.
func TestGroupsOmittedWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := SaveStore(path, Store{Projects: []Project{{Name: "inbox"}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(b), `"groups"`) {
		t.Errorf("store without groups grew a groups key: %s", b)
	}
}
