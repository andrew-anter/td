/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"todo/task"
)

func animTestModel(t *testing.T) uiModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
		{Text: "bread", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	return m
}

// Completing from the pending view must save immediately but keep the
// row on screen while the strikethrough sweeps, hand the cursor to the
// next task, and only then drop the row and flash the completed tab.
func TestCompletionAnimSweepsThenDeparts(t *testing.T) {
	m := animTestModel(t)
	path := m.path

	updated, cmd := m.handleKey(keyMsg("x"))
	m = updated.(uiModel)
	if cmd == nil {
		t.Fatal("completing a task returned no animation command")
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if !got.Project("inbox").Items[0].Done {
		t.Fatal("toggle was not persisted before the animation")
	}

	// The ghost of the completed row holds its old slot; the cursor
	// moved on to the next pending task.
	items := m.list.Items()
	if len(items) != 2 {
		t.Fatalf("ghost row vanished instantly: %d rows, want 2", len(items))
	}
	if items[0].(uiItem).row.fileIdx != 0 {
		t.Errorf("ghost not at the toggled row's slot: %+v", items[0])
	}
	if sel, ok := m.selected(); !ok || sel.row.item.Text != "bread" {
		t.Errorf("cursor did not move to the next task: %+v", sel)
	}

	// Mid-sweep: partially struck, not yet departing.
	m.anim.start = time.Now().Add(-animSweep / 2)
	updated, _ = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	ghost := m.list.Items()[0].(uiItem)
	if ghost.row.strike <= 0 || ghost.row.strike >= 1 {
		t.Errorf("mid-sweep strike = %v, want strictly between 0 and 1", ghost.row.strike)
	}
	if ghost.row.exit != 0 {
		t.Errorf("row started departing during the sweep: exit=%v", ghost.row.exit)
	}

	// Mid-exit: fully struck and fading out.
	m.anim.start = time.Now().Add(-animSweep - animHold - animExit/2)
	updated, _ = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	ghost = m.list.Items()[0].(uiItem)
	if ghost.row.strike != 1 || ghost.row.exit <= 0 || ghost.row.exit >= 1 {
		t.Errorf("mid-exit strike/exit = %v/%v, want 1 and (0,1)", ghost.row.strike, ghost.row.exit)
	}

	// Past the end: the row is gone, the completed tab flashes, and
	// ticks keep running until the flash expires.
	m.anim.start = time.Now().Add(-(animSweep + animHold + animExit))
	updated, cmd = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	if m.anim != nil {
		t.Fatal("animation did not finish")
	}
	items = m.list.Items()
	if len(items) != 1 || items[0].(uiItem).row.item.Text != "bread" {
		t.Fatalf("ghost survived the animation: %+v", items)
	}
	if !time.Now().Before(m.doneFlash) {
		t.Error("completed tab did not flash when the row landed")
	}
	if cmd == nil {
		t.Error("flash needs a tick to expire, got no command")
	}

	m.doneFlash = time.Now().Add(-time.Millisecond)
	updated, cmd = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	if cmd != nil {
		t.Error("ticks kept running after the flash expired")
	}
}

// In the all view the row belongs on screen: only the sweep plays and
// the completed row stays, fully struck, when the animation ends.
func TestCompletionAnimInAllViewKeepsRow(t *testing.T) {
	m := animTestModel(t)
	pressKeys(&m, []string{"tab"}) // pending → all
	if m.view != viewAll {
		t.Fatalf("tab left view %v, want all", m.view)
	}

	updated, cmd := m.handleKey(keyMsg("x"))
	m = updated.(uiModel)
	if cmd == nil || m.anim == nil {
		t.Fatal("completing in the all view started no sweep")
	}
	m.anim.start = time.Now().Add(-(animSweep + animHold))
	updated, _ = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	if m.anim != nil {
		t.Fatal("sweep did not finish in the all view")
	}
	items := m.list.Items()
	if len(items) != 2 {
		t.Fatalf("all view lost a row: %d, want 2", len(items))
	}
	var row uiRow
	for _, li := range items {
		if r := li.(uiItem).row; r.fileIdx == 0 {
			row = r
		}
	}
	if !row.item.Done || row.strike != 0 || row.exit != 0 {
		t.Errorf("finished row not rendered as static done: %+v", row)
	}
}

// A second mutation while the ghost is on screen retires it cleanly.
func TestCompletionAnimCancelledByNextMutation(t *testing.T) {
	m := animTestModel(t)
	updated, _ := m.handleKey(keyMsg("x"))
	m = updated.(uiModel)
	if m.anim == nil {
		t.Fatal("no animation to cancel")
	}
	pressKeys(&m, []string{"d", "y"}) // delete the next task
	if m.anim != nil {
		t.Error("delete left a stale animation running")
	}
	for _, li := range m.list.Items() {
		if li.(uiItem).row.fileIdx == 0 {
			t.Error("ghost row survived an explicit refresh")
		}
	}
}

// Completing the LAST displayed row must leave the cursor on a real
// row: the list clamps pages but not the cursor, so the refresh has to
// pull it back inside the shrunken list.
func TestCompletionLastRowClampsCursor(t *testing.T) {
	m := animTestModel(t)
	pressKeys(&m, []string{"j"}) // cursor on bread, the last row
	updated, _ := m.handleKey(keyMsg("x"))
	m = updated.(uiModel)
	if sel, ok := m.selected(); !ok || sel.row.item.Text != "milk" {
		t.Fatalf("cursor after completing the last row = %+v, want milk", sel)
	}
	m.anim.start = time.Now().Add(-(animSweep + animHold + animExit))
	updated, _ = m.Update(doneAnimTickMsg{})
	m = updated.(uiModel)
	if sel, ok := m.selected(); !ok || sel.row.item.Text != "milk" {
		t.Fatalf("cursor after the animation = %+v, want milk", sel)
	}
}
