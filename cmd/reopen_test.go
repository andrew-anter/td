/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"todo/task"
)

func writeItems(t *testing.T, items []task.Item) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	if err := task.SaveItems(path, items); err != nil {
		t.Fatalf("SaveItems: %v", err)
	}
	viper.Set("datafile", path)
}

func TestReopenPendingItem_NoOp(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "buy milk", Priority: 2, Done: false},
	})

	if err := reopenRun(nil, []string{"1"}); err != nil {
		t.Fatalf("reopenRun: %v", err)
	}

	got, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if got[0].Done {
		t.Errorf("pending item marked done after reopen")
	}
}

func TestReopenDoneItem_MarksPending(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "buy milk", Priority: 2, Done: false},
		{Text: "write report", Priority: 1, Done: true},
	})

	// ByPri order: pending first (buy milk, idx 0), then done (write
	// report, idx 1). So "write report" is at sortedOrderIndices[1],
	// meaning "td reopen 2" reopens it.
	if err := reopenRun(nil, []string{"2"}); err != nil {
		t.Fatalf("reopenRun: %v", err)
	}

	got, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if got[0].Done {
		t.Errorf("item #0 (buy milk) should remain pending")
	}
	if got[1].Done {
		t.Errorf("item #1 (write report) should now be pending")
	}
}

func TestReopenMissingArg(t *testing.T) {
	writeItems(t, nil)
	if err := reopenRun(nil, nil); err == nil {
		t.Errorf("expected error when no item number given")
	}
}

func TestReopenInvalidIndex(t *testing.T) {
	writeItems(t, []task.Item{{Text: "x", Priority: 2, Done: true}})
	if err := reopenRun(nil, []string{"5"}); err == nil {
		t.Errorf("expected error for out-of-range index")
	}
	if err := reopenRun(nil, []string{"abc"}); err == nil {
		t.Errorf("expected error for non-numeric index")
	}
}
