/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"todo/task"
)

func setDeleteCompleted(t *testing.T, v bool) {
	t.Helper()
	prev := deleteCompletedOpt
	deleteCompletedOpt = v
	t.Cleanup(func() { deleteCompletedOpt = prev })
}

func readSaved(t *testing.T) []task.Item {
	t.Helper()
	got, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	return got
}

// The number shown next to a completed item in `list --json
// --completed` must delete exactly that item — pending items occupy
// the leading numbers in the global order.
func TestDeleteNumberFromCompletedView(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "milk", Priority: 2, Done: false},
		{Text: "urgent call", Priority: 1, Done: false},
		{Text: "old task", Priority: 2, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true, ShowCompleted: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	var rows []struct {
		Index int    `json:"Index"`
		Text  string `json:"Text"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if len(rows) != 1 || rows[0].Text != "old task" {
		t.Fatalf("completed view = %+v, want one row 'old task'", rows)
	}

	num := strconv.Itoa(rows[0].Index)
	want := rows[0].Text

	if _, err := captureStdout(t, func() error {
		return deleteRun(nil, []string{num})
	}); err != nil {
		t.Fatalf("deleteRun(%s): %v", num, err)
	}

	got := readSaved(t)
	if len(got) != 2 {
		t.Fatalf("got %d items after delete, want 2: %+v", len(got), got)
	}
	for _, it := range got {
		if it.Text == want {
			t.Errorf("completed item %q should have been deleted", want)
		}
	}
}

// Same invariant for reopen: reopening the number shown in the
// completed view reopens that item, not the first pending one.
func TestReopenNumberFromCompletedView(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "milk", Priority: 2, Done: false},
		{Text: "urgent call", Priority: 1, Done: false},
		{Text: "old task", Priority: 2, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true, ShowCompleted: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	var rows []struct {
		Index int    `json:"Index"`
		Text  string `json:"Text"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}

	if _, err := captureStdout(t, func() error {
		return reopenRun(nil, []string{strconv.Itoa(rows[0].Index)})
	}); err != nil {
		t.Fatalf("reopenRun: %v", err)
	}

	got := readSaved(t)
	for _, it := range got {
		if it.Text == "old task" && it.Done {
			t.Errorf(`"old task" should be reopened by number %d`, rows[0].Index)
		}
	}
}

func TestDeleteCompletedRemovesOnlyDone(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "keep", Priority: 2, Done: false},
		{Text: "gone-a", Priority: 2, Done: true},
		{Text: "gone-b", Priority: 1, Done: true},
	})
	setDeleteCompleted(t, true)

	if _, err := captureStdout(t, func() error {
		return deleteRun(nil, nil)
	}); err != nil {
		t.Fatalf("deleteRun --completed: %v", err)
	}

	got := readSaved(t)
	if len(got) != 1 || got[0].Text != "keep" {
		t.Errorf("after delete --completed got %+v, want only %q", got, "keep")
	}
}

func TestDeleteCompletedRejectsItemNumber(t *testing.T) {
	writeItems(t, []task.Item{{Text: "x", Priority: 2, Done: true}})
	setDeleteCompleted(t, true)

	if err := deleteRun(nil, []string{"1"}); err == nil {
		t.Errorf("expected error when item number given with --completed")
	}
}
