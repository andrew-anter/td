/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"path/filepath"
	"testing"

	"todo/task"
)

type rowPair struct{ num, fileIdx int }

func pairs(rows []uiRow) []rowPair {
	out := make([]rowPair, len(rows))
	for k, r := range rows {
		out[k] = rowPair{r.num, r.fileIdx}
	}
	return out
}

func equalPairs(a, b []rowPair) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

// The UI's row numbers must match the CLI's global numbering exactly,
// or numbers shown in the TUI would address the wrong item.
func TestVisibleRowsNumberingMatchesCLI(t *testing.T) {
	items := []task.Item{
		{Text: "milk", Priority: 2, Done: false},        // idx 0
		{Text: "urgent call", Priority: 1, Done: false}, // idx 1
		{Text: "old task A", Priority: 2, Done: true},   // idx 2
		{Text: "write blog", Priority: 3, Done: false},  // idx 3
		{Text: "old task B", Priority: 2, Done: true},   // idx 4
	}

	// Full sorted order: urgent call(1), milk(2), write blog(3),
	// old task A(4), old task B(5).
	cases := []struct {
		name string
		view viewMode
		want []rowPair
	}{
		{"pending", viewPending, []rowPair{{1, 1}, {2, 0}, {3, 3}}},
		{"done", viewDone, []rowPair{{4, 2}, {5, 4}}},
		{"all", viewAll, []rowPair{{1, 1}, {2, 0}, {3, 3}, {4, 2}, {5, 4}}},
	}
	for _, tc := range cases {
		got := pairs(visibleRows(items, tc.view))
		if !equalPairs(got, tc.want) {
			t.Errorf("%s: rows = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// A number shown in the TUI for any view must target the same item as
// the CLI's sortedOrderIndices — the invariant every view must keep.
func TestVisibleRowFileIdxAgreesWithSortedOrderIndices(t *testing.T) {
	items := []task.Item{
		{Text: "a", Priority: 3, Done: false},
		{Text: "b", Priority: 1, Done: true},
		{Text: "c", Priority: 2, Done: true},
		{Text: "d", Priority: 1, Done: false},
		{Text: "e", Priority: 2, Done: false},
	}
	order := sortedOrderIndices(items)
	for _, view := range []viewMode{viewPending, viewAll, viewDone} {
		for _, r := range visibleRows(items, view) {
			if order[r.num-1] != r.fileIdx {
				t.Errorf("view %v: row num %d has fileIdx %d, CLI expects %d",
					view, r.num, r.fileIdx, order[r.num-1])
			}
		}
	}
}

func TestToggleAndDeleteItem(t *testing.T) {
	items := []task.Item{
		{Text: "keep", Priority: 2, Done: false},
		{Text: "flip", Priority: 2, Done: false},
	}
	toggleItem(items, 1)
	if !items[1].Done || items[0].Done {
		t.Errorf("toggleItem hit the wrong element: %+v", items)
	}
	items = deleteItem(items, 0)
	if len(items) != 1 || items[0].Text != "flip" {
		t.Errorf("deleteItem removed the wrong element: %+v", items)
	}
}

// Round-tripping the model's snapshot through SaveStore/ReadStore is
// what persist() does on every mutation.
func TestUIPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{
		Name:  task.DefaultProject,
		Items: []task.Item{{Text: "one", Priority: 2, Done: false}},
	}}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = append(m.items, task.Item{Text: "two", Priority: 1, Done: true})
	m.persist("added")
	if m.status != "added" {
		t.Fatalf("persist set status %q, want %q", m.status, "added")
	}

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	p := got.Project(task.DefaultProject)
	if p == nil || len(p.Items) != 2 || p.Items[1].Text != "two" || !p.Items[1].Done {
		t.Fatalf("saved file = %+v", got)
	}
}

// The project switcher must cycle through the store's namespaces in
// order, wrapping both ways, and carry each project's own items.
func TestSwitchProjectCycles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "i", Priority: 2}}},
		{Name: "work", Items: []task.Item{{Text: "w", Priority: 2}}},
	}}
	m := newUIModel(path, store, "inbox")

	m.switchProject(1)
	if m.project != "work" || len(m.items) != 1 || m.items[0].Text != "w" {
		t.Fatalf("after next: project=%q items=%+v", m.project, m.items)
	}
	m.switchProject(1)
	if m.project != "inbox" {
		t.Errorf("forward wrap-around failed: project=%q", m.project)
	}
	m.switchProject(-1)
	if m.project != "work" {
		t.Errorf("backward wrap-around failed: project=%q", m.project)
	}
}

// persist() must write the active project's items and leave every
// other project in the store untouched.
func TestPersistKeepsOtherProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "keep me", Priority: 2}}},
	}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = append(m.items, task.Item{Text: "new inbox item", Priority: 2})
	m.persist("added")

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if w := got.Project("work"); w == nil || len(w.Items) != 1 || w.Items[0].Text != "keep me" {
		t.Errorf("work project clobbered: %+v", got)
	}
	in := got.Project(task.DefaultProject)
	if in == nil || len(in.Items) != 1 || in.Items[0].Text != "new inbox item" {
		t.Errorf("inbox persist wrong: %+v", in)
	}
}

// createProject validates, adds, switches to, and persists a new
// namespace; duplicates and invalid names are refused without
// touching the store.
func TestCreateProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	m := newUIModel(path, task.Store{}, task.DefaultProject)

	if err := m.createProject("gym"); err != nil {
		t.Fatalf("createProject: %v", err)
	}
	if m.project != "gym" || m.status != `created project "gym"` {
		t.Fatalf("after create: project=%q status=%q", m.project, m.status)
	}
	if err := m.createProject("gym"); err == nil {
		t.Errorf("expected duplicate project error")
	}
	if err := m.createProject("two words"); err == nil {
		t.Errorf("expected whitespace name error")
	}

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	names := got.ProjectNames()
	if len(names) != 2 || names[0] != task.DefaultProject || names[1] != "gym" {
		t.Errorf("persisted projects = %v, want [inbox gym]", names)
	}
}

// setProject must swap both the name and the item slice the views and
// mutations operate on.
func TestSetProjectSwapsItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "a", Items: []task.Item{{Text: "in a", Priority: 2}}},
		{Name: "b", Items: []task.Item{{Text: "in b1", Priority: 2}, {Text: "in b2", Priority: 2}}},
	}}
	m := newUIModel(path, store, "a")
	if len(m.items) != 1 {
		t.Fatalf("model items = %+v", m.items)
	}
	m.setProject("b")
	if m.project != "b" || len(m.items) != 2 || m.items[0].Text != "in b1" {
		t.Errorf("after setProject(b): project=%q items=%+v", m.project, m.items)
	}
	m.setProject("fresh")
	if m.project != "fresh" || len(m.items) != 0 {
		t.Errorf("setProject on a missing name should give an empty namespace: %+v", m.items)
	}
}

// After deleting the top pending item, the survivor is renumbered to
// 1 and stays addressable in the same view.
func TestVisibleRowsAfterDeleteRenumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{
		Name: task.DefaultProject,
		Items: []task.Item{
			{Text: "a", Priority: 1, Done: false},
			{Text: "b", Priority: 2, Done: false},
		},
	}}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = deleteItem(m.items, m.visibleRows()[0].fileIdx)
	m.refresh("deleted")

	got := pairs(m.visibleRows())
	want := []rowPair{{1, 0}} // "b" now first in file and display order
	if !equalPairs(got, want) {
		t.Fatalf("rows after delete = %+v, want %+v", got, want)
	}
	if m.items[0].Text != "b" {
		t.Errorf("fileIdx 0 should now be %q, got %q", "b", m.items[0].Text)
	}
}
