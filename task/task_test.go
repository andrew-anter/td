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
	}{
		{1, 1},
		{3, 3},
		{2, 2},
		{0, 2},
		{4, 2},
		{-1, 2},
	}
	for _, c := range cases {
		var it Item
		it.SetPriority(c.in)
		if it.Priority != c.want {
			t.Errorf("SetPriority(%d) = %d, want %d", c.in, it.Priority, c.want)
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

func TestSaveAndReadItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	in := []Item{
		{Text: "first", Priority: 1, Done: false},
		{Text: "second", Priority: 3, Done: true},
		{Text: "third", Priority: 2, Done: false},
	}

	if err := SaveItems(path, in); err != nil {
		t.Fatalf("SaveItems: %v", err)
	}

	out, err := ReadItems(path)
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

func TestSaveItemsRejectsBadPath(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "does", "not", "exist", "tasks.json")
	if err := SaveItems(bad, []Item{{Text: "x"}}); err == nil {
		t.Fatalf("expected error saving into non-existent directory")
	}
	if _, err := os.Stat(bad); err == nil {
		t.Errorf("partial file was created at %s", bad)
	}
}
