/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"todo/task"
)

func visualTestModel(t *testing.T) uiModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
		{Text: "bread", Priority: 2},
		{Text: "eggs", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	return m
}

func visualFlags(m uiModel) []bool {
	out := make([]bool, len(m.list.Items()))
	for i, li := range m.list.Items() {
		out[i] = li.(uiItem).row.visual
	}
	return out
}

// v anchors, j extends, x completes the whole range in one save and
// hands the cursor back to a real row.
func TestVisualSelectBatchComplete(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"v", "j"})
	if m.mode != modeVisual || m.visualAnchor != 0 {
		t.Fatalf("v/j left mode %v anchor %d, want visual/0", m.mode, m.visualAnchor)
	}
	if flags := visualFlags(m); !flags[0] || !flags[1] || flags[2] {
		t.Fatalf("selection flags = %v, want first two rows", flags)
	}
	if lo, hi := m.visualRange(); lo != 0 || hi != 1 {
		t.Fatalf("range = %d..%d, want 0..1", lo, hi)
	}

	updated, cmd := m.handleKey(keyMsg("x"))
	m = updated.(uiModel)
	if m.mode != modeNormal || m.visualAnchor != -1 {
		t.Fatalf("batch x left mode %v anchor %d, want normal/-1", m.mode, m.visualAnchor)
	}
	if !m.items[0].Done || !m.items[1].Done || m.items[2].Done {
		t.Fatalf("batch x hit the wrong items: %+v", m.items)
	}
	if m.status != "2 tasks → done" {
		t.Errorf("status = %q", m.status)
	}
	if cmd == nil {
		t.Error("batch completion did not flash the completed tab")
	}
	if _, ok := m.selected(); !ok {
		t.Error("cursor left without a valid row after batch completion")
	}
	got, err := task.ReadStore(m.path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	items := got.Project("inbox").Items
	if !items[0].Done || !items[1].Done || items[2].Done {
		t.Errorf("batch completion did not persist: %+v", items)
	}
	for _, f := range visualFlags(m) {
		if f {
			t.Error("selection highlight survived the batch action")
		}
	}
}

// Selecting upward from the anchor works, and x on an all-done
// selection reopens it instead of toggling row by row.
func TestVisualUpwardSelectionReopens(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"j", "v", "k"})
	if lo, hi := m.visualRange(); lo != 0 || hi != 1 {
		t.Fatalf("upward range = %d..%d, want 0..1", lo, hi)
	}
	pressKeys(&m, []string{"x"})   // complete both
	pressKeys(&m, []string{"tab"}) // all view: done rows stay visible
	pressKeys(&m, []string{"j", "j", "v", "k", "x"})
	if m.items[0].Done || m.items[1].Done {
		t.Fatalf("x on a done selection did not reopen: %+v", m.items)
	}
	if m.status != "2 tasks → pending" {
		t.Errorf("status = %q", m.status)
	}
}

// d asks once for the whole range; y deletes every selected row, n
// leaves them all alone and clears the highlight.
func TestVisualBatchDeleteConfirm(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"v", "j", "d"})
	if m.mode != modeConfirm || m.confirm == nil {
		t.Fatalf("batch d opened mode %v, want confirm", m.mode)
	}
	if !strings.Contains(m.confirm.prompt(), "delete 2 tasks?") {
		t.Errorf("prompt = %q, want a count", m.confirm.prompt())
	}
	pressKeys(&m, []string{"y"})
	if len(m.items) != 1 || m.items[0].Text != "eggs" {
		t.Fatalf("y deleted the wrong rows: %+v", m.items)
	}
	got, err := task.ReadStore(m.path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if p := got.Project("inbox"); len(p.Items) != 1 || p.Items[0].Text != "eggs" {
		t.Errorf("batch delete did not persist: %+v", p)
	}

	m = visualTestModel(t)
	pressKeys(&m, []string{"v", "j", "d", "n"})
	if m.mode != modeNormal || len(m.items) != 3 {
		t.Fatalf("n left mode %v with %d items, want normal/3", m.mode, len(m.items))
	}
	for _, f := range visualFlags(m) {
		if f {
			t.Error("cancelled delete left the selection highlighted")
		}
	}
}

// 1-3 set the priority of every selected row.
func TestVisualBatchPriority(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"v", "j", "1"})
	if m.items[0].Priority != 1 || m.items[1].Priority != 1 || m.items[2].Priority != 2 {
		t.Fatalf("batch priority hit the wrong rows: %+v", m.items)
	}
	if m.status != "2 tasks → priority 1" {
		t.Errorf("status = %q", m.status)
	}
}

// m carries the whole selection into the projects screen and moves it
// on enter.
func TestVisualBatchMove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{
			{Text: "milk", Priority: 2},
			{Text: "bread", Priority: 2},
			{Text: "eggs", Priority: 2},
		}},
		{Name: "work"},
	}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	pressKeys(&m, []string{"v", "j", "m"})
	if m.mode != modeProjects || !m.projectsForMove || len(m.moveFileIdxs) != 2 {
		t.Fatalf("batch m opened mode %v move=%v idxs=%v", m.mode, m.projectsForMove, m.moveFileIdxs)
	}
	pressKeys(&m, []string{"enter"})
	if m.mode != modeNormal {
		t.Fatalf("enter left mode %v, want normal", m.mode)
	}
	if len(m.items) != 1 || m.items[0].Text != "eggs" {
		t.Fatalf("source kept the wrong rows: %+v", m.items)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if p := got.Project("work"); p == nil || len(p.Items) != 2 {
		t.Fatalf("batch move did not land in work: %+v", got)
	}
	if m.status != `moved 2 tasks to "work"` {
		t.Errorf("status = %q", m.status)
	}
}

// esc drops the selection without touching the items; a single-item
// verb leaves visual mode first and acts on the cursor row.
func TestVisualCancelAndSingleVerbFallThrough(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"v", "j", "esc"})
	if m.mode != modeNormal || m.visualAnchor != -1 {
		t.Fatalf("esc left mode %v anchor %d", m.mode, m.visualAnchor)
	}
	for _, f := range visualFlags(m) {
		if f {
			t.Error("esc left the selection highlighted")
		}
	}

	pressKeys(&m, []string{"v", "e"})
	if m.mode != modeEdit || m.editingFileIdx != 1 {
		t.Fatalf("e in visual mode opened mode %v on fileIdx %d, want edit/1", m.mode, m.editingFileIdx)
	}
}

// Entering visual mode retires an in-flight completion ghost so the
// selection addresses only real rows.
func TestVisualEntryRetiresAnimation(t *testing.T) {
	m := visualTestModel(t)
	pressKeys(&m, []string{"x"})
	if m.anim == nil {
		t.Fatal("no animation to retire")
	}
	pressKeys(&m, []string{"v"})
	if m.anim != nil {
		t.Error("visual mode kept a stale animation")
	}
	for _, li := range m.list.Items() {
		if r := li.(uiItem).row; r.strike > 0 || r.exit > 0 {
			t.Error("ghost row survived into visual mode")
		}
	}
}
