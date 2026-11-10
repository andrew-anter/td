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

// Round-tripping the model's snapshot through SaveItems/ReadItems is
// what persist() does on every mutation.
func TestUIPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	items := []task.Item{{Text: "one", Priority: 2, Done: false}}
	m := newUIModel(path, items)
	m.items = append(m.items, task.Item{Text: "two", Priority: 1, Done: true})
	m.persist("added")
	if m.status != "added" {
		t.Fatalf("persist set status %q, want %q", m.status, "added")
	}

	got, err := task.ReadItems(path)
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if len(got) != 2 || got[1].Text != "two" || !got[1].Done {
		t.Fatalf("saved file = %+v", got)
	}
}

// After deleting the top pending item, the survivor is renumbered to
// 1 and stays addressable in the same view.
func TestVisibleRowsAfterDeleteRenumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	m := newUIModel(path, []task.Item{
		{Text: "a", Priority: 1, Done: false},
		{Text: "b", Priority: 2, Done: false},
	})
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
