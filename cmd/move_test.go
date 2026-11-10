/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"strings"
	"testing"

	"todo/task"
)

// The number given to move resolves against the source project's
// sorted display order, and the item keeps its priority and done
// state in the target.
func TestMoveItemBetweenProjects(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{
			{Text: "report", Priority: 2, Done: true},
			{Text: "milk", Priority: 1, Done: false},
		}},
		{Name: "gym", Items: []task.Item{{Text: "squat", Priority: 2}}},
	}})
	setProject(t, "inbox")

	// Sorted inbox: 1. milk (pending, P1), 2. report (done).
	out, err := captureStdout(t, func() error {
		return moveRun(nil, []string{"2", "gym"})
	})
	if err != nil {
		t.Fatalf("moveRun: %v", err)
	}
	if !strings.Contains(out, `"report" moved to "gym"`) {
		t.Errorf("output = %q", out)
	}

	got := readStore(t)
	in := got.Project("inbox")
	if in == nil || len(in.Items) != 1 || in.Items[0].Text != "milk" {
		t.Errorf("inbox = %+v, want only milk", in)
	}
	g := got.Project("gym")
	if g == nil || len(g.Items) != 2 {
		t.Fatalf("gym = %+v, want two items", g)
	}
	moved := g.Items[1]
	if moved.Text != "report" || moved.Priority != 2 || !moved.Done {
		t.Errorf("moved item lost state: %+v", moved)
	}
}

func TestMoveRejectsBadTargets(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "milk", Priority: 1}}},
		{Name: "gym", Items: nil},
	}})
	setProject(t, "inbox")

	if err := moveRun(nil, []string{"1", "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "td project create ghost") {
		t.Errorf("unknown target: got %v, want no-such-project hint", err)
	}
	if err := moveRun(nil, []string{"1", "inbox"}); err == nil ||
		!strings.Contains(err.Error(), "already in project") {
		t.Errorf("same project: got %v, want already-in error", err)
	}
	if err := moveRun(nil, []string{"5", "gym"}); err == nil {
		t.Errorf("out-of-range number: expected error")
	}
	if err := moveRun(nil, []string{"abc", "gym"}); err == nil {
		t.Errorf("non-numeric label: expected error")
	}
}
