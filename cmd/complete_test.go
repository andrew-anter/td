/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"testing"
	"todo/task"
)

// Completing an item stamps Completed once; completing it again must
// not move the timestamp.
func TestCompleteStampsOnce(t *testing.T) {
	writeItems(t, []task.Item{{Text: "ship it", Priority: 2}})

	if err := completeRun(nil, []string{"1"}); err != nil {
		t.Fatalf("completeRun: %v", err)
	}
	first := readSaved(t)[0]
	if !first.Done {
		t.Fatalf("item not marked done: %+v", first)
	}
	if first.Completed.IsZero() {
		t.Fatalf("Completed not stamped: %+v", first)
	}

	if err := completeRun(nil, []string{"1"}); err != nil {
		t.Fatalf("second completeRun: %v", err)
	}
	if second := readSaved(t)[0]; !second.Completed.Equal(first.Completed) {
		t.Errorf("Completed moved on re-complete: %v -> %v", first.Completed, second.Completed)
	}
}
