/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"path/filepath"
	"testing"

	"todo/task"
)

func twoProjectStore() task.Store {
	return task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "mail", Priority: 2}}},
		{Name: "gym", Items: []task.Item{
			{Text: "squat", Priority: 2},
			{Text: "bench", Priority: 1},
			{Text: "stretch", Priority: 2, Done: true},
		}},
	}}
}

func TestUIStateRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	f := filepath.Join(t.TempDir(), "tasks.json")

	if _, ok := readUIState(f); ok {
		t.Fatalf("state exists before any write")
	}
	writeUIState(f, uiState{Project: "gym", View: "all", Task: "squat"})
	st, ok := readUIState(f)
	if !ok || st.Project != "gym" || st.View != "all" || st.Task != "squat" {
		t.Fatalf("round-trip = %+v", st)
	}

	g := filepath.Join(t.TempDir(), "other.json")
	writeUIState(g, uiState{Project: "inbox"})
	if st2, _ := readUIState(f); st2.Project != "gym" {
		t.Errorf("writing a second data file clobbered the first's state")
	}
}

func TestRestoreUIState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	m := newUIModel(path, twoProjectStore(), "inbox")

	restoreUIState(&m, uiState{Project: "gym", View: "all", Task: "squat"}, false)

	if m.project != "gym" {
		t.Errorf("project = %q, want gym", m.project)
	}
	if m.view != viewAll {
		t.Errorf("view = %v, want all", m.view)
	}
	if it, ok := m.selected(); !ok || it.row.item.Text != "squat" {
		t.Errorf("cursor = %+v, want on squat", it)
	}
}

func TestRestoreUIStatePrecedenceAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")

	m := newUIModel(path, twoProjectStore(), "inbox")
	restoreUIState(&m, uiState{Project: "gym", View: "bogus", Task: "nonexistent"}, true)
	if m.project != "inbox" {
		t.Errorf("explicit -P lost to stored project %q", m.project)
	}
	if m.view != viewPending {
		t.Errorf("unknown view changed the tab to %v", m.view)
	}
	if it, ok := m.selected(); !ok || it.row.item.Text != "mail" {
		t.Errorf("missing task text moved the cursor off the first row: %+v", it)
	}

	g := newUIModel(path, twoProjectStore(), "inbox")
	restoreUIState(&g, uiState{Project: "deleted", View: "completed"}, false)
	if g.project != "inbox" || g.view != viewDone {
		t.Errorf("deleted project = %q, view = %v", g.project, g.view)
	}
}

func TestUIStateFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	m := newUIModel(path, twoProjectStore(), "gym")
	m.view = viewDone
	m.refresh("")

	st := uiStateFrom(m)
	if st.Project != "gym" || st.View != "completed" || st.Task != "stretch" {
		t.Errorf("uiStateFrom = %+v", st)
	}
}
