/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package task

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
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
	ok := []string{"work", "home-chores", "v2", "a1"}
	bad := []string{"", " ", "two words", "tab\there", "trail "}
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
