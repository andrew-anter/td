/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"todo/task"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan struct{})
	var out []byte
	go func() {
		out, _ = io.ReadAll(r)
		close(done)
	}()

	fnErr := fn()
	w.Close()
	<-done
	os.Stdout = old
	return string(out), fnErr
}

// writeDataFile writes items to a temp file and points viper at it.
// Tests that call this must not use t.Parallel().
func writeDataFile(t *testing.T, items []task.Item) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	if err := task.SaveItems(path, items); err != nil {
		t.Fatalf("SaveItems: %v", err)
	}
	viper.Set("datafile", path)
}

func TestRunListCount_AllPending(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "a", Priority: 2, Done: false},
		{Text: "b", Priority: 1, Done: false},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsCount: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got struct {
		Pending   int `json:"pending"`
		Completed int `json:"completed"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := struct{ Pending, Completed, Total int }{2, 0, 2}
	if got.Pending != want.Pending || got.Completed != want.Completed || got.Total != want.Total {
		t.Errorf("count = %+v, want %+v", got, want)
	}
}

func TestRunListCount_Mixed(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "a", Priority: 2, Done: false},
		{Text: "b", Priority: 1, Done: true},
		{Text: "c", Priority: 3, Done: false},
		{Text: "d", Priority: 2, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsCount: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got struct {
		Pending   int `json:"pending"`
		Completed int `json:"completed"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := struct{ Pending, Completed, Total int }{2, 2, 4}
	if got.Pending != want.Pending || got.Completed != want.Completed || got.Total != want.Total {
		t.Errorf("count = %+v, want %+v", got, want)
	}
}

func TestRunListCount_IgnoresFilters(t *testing.T) {
	// --count must report totals across the whole data file even when
	// filter flags are also set.
	writeDataFile(t, []task.Item{
		{Text: "a", Priority: 2, Done: false},
		{Text: "b", Priority: 1, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsCount: true, ShowCompleted: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got struct {
		Pending   int `json:"pending"`
		Completed int `json:"completed"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := struct{ Pending, Completed, Total int }{1, 1, 2}
	if got.Pending != want.Pending || got.Completed != want.Completed || got.Total != want.Total {
		t.Errorf("count = %+v, want %+v", got, want)
	}
}

func TestRunListJSON_DefaultShowsPending(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "low-pending", Priority: 3, Done: false},
		{Text: "done-item", Priority: 1, Done: true},
		{Text: "high-pending", Priority: 1, Done: false},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got []task.Item
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}

	// Default filter: pending only. The wrapper in runList strips
	// Index, so this is a flat array of items in display order.
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(got), got)
	}
	if got[0].Text != "high-pending" {
		t.Errorf("first item = %q, want %q (priority 1 sorts first)", got[0].Text, "high-pending")
	}
	if got[1].Text != "low-pending" {
		t.Errorf("second item = %q, want %q", got[1].Text, "low-pending")
	}
	if got[0].Done || got[1].Done {
		t.Errorf("expected only pending items, got %+v", got)
	}
}

func TestRunListJSON_IncludesIndexAndOrder(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "z", Priority: 2, Done: false},
		{Text: "a", Priority: 1, Done: false},
		{Text: "m", Priority: 3, Done: false},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	// Use a local shape with Index so we can assert the wrapper field
	// without leaking it into task.Item.
	var got []struct {
		Index    int    `json:"Index"`
		Text     string `json:"Text"`
		Priority int    `json:"Priority"`
		Done     bool   `json:"Done"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d items, want 3", len(got))
	}
	want := []string{"a", "z", "m"}
	for i, w := range want {
		if got[i].Text != w {
			t.Errorf("item %d text = %q, want %q", i, got[i].Text, w)
		}
		if got[i].Index != i+1 {
			t.Errorf("item %d Index = %d, want %d", i, got[i].Index, i+1)
		}
	}
}

func TestRunListJSON_CompletedFilter(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "pending", Priority: 1, Done: false},
		{Text: "done-a", Priority: 2, Done: true},
		{Text: "done-b", Priority: 3, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true, ShowCompleted: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got []task.Item
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(got), got)
	}
	for _, it := range got {
		if !it.Done {
			t.Errorf("expected only completed items, got %+v", got)
		}
	}
}

func TestRunListJSON_AllFilter(t *testing.T) {
	writeDataFile(t, []task.Item{
		{Text: "pending", Priority: 1, Done: false},
		{Text: "done", Priority: 1, Done: true},
	})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true, ShowAll: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got []task.Item
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
}

func TestRunListJSON_Empty(t *testing.T) {
	writeDataFile(t, nil)

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	var got []task.Item
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if len(got) != 0 {
		t.Errorf("got %d items, want 0", len(got))
	}
}
