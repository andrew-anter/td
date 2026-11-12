/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"strings"
	"testing"

	"todo/task"
)

func TestDescribeSetsAndClears(t *testing.T) {
	writeItems(t, []task.Item{{Text: "fix the sink", Priority: 2}})

	if err := describeRun(nil, []string{"1", "kerl valve stripped"}); err != nil {
		t.Fatalf("describeRun set: %v", err)
	}
	if got := readSaved(t)[0].Description; got != "kerl valve stripped" {
		t.Fatalf("Description = %q, want %q", got, "kerl valve stripped")
	}

	if err := describeRun(nil, []string{"1", ""}); err != nil {
		t.Fatalf("describeRun clear: %v", err)
	}
	if got := readSaved(t)[0]; got.Description != "" {
		t.Fatalf("Description not cleared: %+v", got)
	}
}

// Display numbers resolve against sorted order, not file order.
func TestDescribeUsesDisplayNumbering(t *testing.T) {
	writeItems(t, []task.Item{
		{Text: "low", Priority: 3},
		{Text: "urgent", Priority: 1},
	})

	if err := describeRun(nil, []string{"1", "note"}); err != nil {
		t.Fatalf("describeRun: %v", err)
	}
	items := readSaved(t)
	if items[1].Text != "urgent" || items[1].Description != "note" {
		t.Fatalf("describe hit the wrong item: %+v", items)
	}
	if items[0].Description != "" {
		t.Fatalf("unrelated item modified: %+v", items[0])
	}
}

func TestDescribeBadNumber(t *testing.T) {
	writeItems(t, []task.Item{{Text: "one", Priority: 2}})

	if err := describeRun(nil, []string{"7", "note"}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v, want an unknown-label error", err)
	}
}
