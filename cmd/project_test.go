/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"todo/task"
)

// setProject points the current project (what --project would set) at
// name for the duration of the test.
func setProject(t *testing.T, name string) {
	t.Helper()
	viper.Set("project", name)
	t.Cleanup(func() { viper.Set("project", "") })
}

func writeStore(t *testing.T, store task.Store) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	if err := task.SaveStore(path, store); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	viper.Set("datafile", path)
	return path
}

func readStore(t *testing.T) task.Store {
	t.Helper()
	s, err := task.ReadStore(viper.GetString("datafile"))
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	return s
}

func TestAddToExplicitProject(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "inbox item", Priority: 2}}},
		{Name: "work", Items: nil},
	}})
	setProject(t, "work")

	if err := addRun(nil, []string{"task A"}); err != nil {
		t.Fatalf("addRun: %v", err)
	}

	got := readStore(t)
	w := got.Project("work")
	if w == nil || len(w.Items) != 1 || w.Items[0].Text != "task A" {
		t.Errorf("work = %+v, want one item %q", w, "task A")
	}
	in := got.Project("inbox")
	if in == nil || len(in.Items) != 1 || in.Items[0].Text != "inbox item" {
		t.Errorf("inbox was disturbed: %+v", in)
	}
}

func TestAddUnknownProjectErrors(t *testing.T) {
	writeStore(t, task.Store{})
	setProject(t, "ghost")

	err := addRun(nil, []string{"nope"})
	if err == nil {
		t.Fatalf("expected error adding to a non-existent project")
	}
	if !strings.Contains(err.Error(), "td project create ghost") {
		t.Errorf("error %q should suggest how to create the project", err)
	}
}

func TestAddDefaultProjectAutoCreated(t *testing.T) {
	writeStore(t, task.Store{})
	setProject(t, "")

	if err := addRun(nil, []string{"first thing"}); err != nil {
		t.Fatalf("addRun: %v", err)
	}

	got := readStore(t)
	p := got.Project(task.DefaultProject)
	if p == nil || len(p.Items) != 1 || p.Items[0].Text != "first thing" {
		t.Errorf("default project = %+v, want the added item", p)
	}
}

func TestProjectCreateAndDuplicate(t *testing.T) {
	writeStore(t, task.Store{})

	if _, err := captureStdout(t, func() error {
		return projectCreateRun(nil, []string{"work"})
	}); err != nil {
		t.Fatalf("projectCreateRun: %v", err)
	}
	if got := readStore(t); got.Project("work") == nil {
		t.Fatalf("project work missing after create")
	}
	if err := projectCreateRun(nil, []string{"work"}); err == nil {
		t.Errorf("expected error creating a duplicate project")
	}
	if err := projectCreateRun(nil, []string{"two words"}); err == nil {
		t.Errorf("expected error for name containing whitespace")
	}
}

func TestProjectDeleteGuardThenForce(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "busy", Priority: 2}}},
		{Name: "empty"},
	}})

	if err := projectDeleteRun(nil, []string{"work"}); err == nil {
		t.Fatalf("expected error deleting a project that has items")
	}
	if err := projectDeleteRun(nil, []string{"ghost"}); err == nil {
		t.Errorf("expected error deleting a non-existent project")
	}

	// Empty projects need no --force.
	if _, err := captureStdout(t, func() error {
		return projectDeleteRun(nil, []string{"empty"})
	}); err != nil {
		t.Fatalf("projectDeleteRun(empty): %v", err)
	}
	if got := readStore(t); got.Project("empty") != nil {
		t.Errorf("empty project still present")
	}

	prev := projectForceOpt
	projectForceOpt = true
	t.Cleanup(func() { projectForceOpt = prev })

	if _, err := captureStdout(t, func() error {
		return projectDeleteRun(nil, []string{"work"})
	}); err != nil {
		t.Fatalf("projectDeleteRun(work, force): %v", err)
	}
	if got := readStore(t); got.Project("work") != nil {
		t.Errorf("work project still present after forced delete")
	}
}

func TestProjectRename(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "keep", Priority: 2}}},
		{Name: "home"},
	}})

	if _, err := captureStdout(t, func() error {
		return projectRenameRun(nil, []string{"work", "job"})
	}); err != nil {
		t.Fatalf("projectRenameRun: %v", err)
	}
	got := readStore(t)
	if got.Project("work") != nil {
		t.Errorf("old name still present: %+v", got)
	}
	p := got.Project("job")
	if p == nil || len(p.Items) != 1 || p.Items[0].Text != "keep" {
		t.Errorf("renamed project lost items: %+v", p)
	}
	if err := projectRenameRun(nil, []string{"job", "home"}); err == nil {
		t.Errorf("expected error renaming onto an existing project")
	}
}

// Item numbers are per-project: completing #1 of one project must not
// touch the item #1 of another, and listing one project must never
// show another's items.
func TestNumberingIsScopedPerProject(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "work-one", Priority: 1}, {Text: "work-two", Priority: 2}}},
		{Name: "home", Items: []task.Item{{Text: "home-one", Priority: 1}}},
	}})
	setProject(t, "work")

	if _, err := captureStdout(t, func() error {
		return completeRun(nil, []string{"1"})
	}); err != nil {
		t.Fatalf("completeRun: %v", err)
	}

	got := readStore(t)
	if w := got.Project("work"); w == nil || !w.Items[0].Done || w.Items[1].Done {
		t.Errorf("work items = %+v, want only work-one done", got.Project("work"))
	}
	if h := got.Project("home"); h == nil || h.Items[0].Done {
		t.Errorf("home item #1 was disturbed: %+v", h)
	}

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsJSON: true, ShowAll: true, Project: "home"})
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
	if len(rows) != 1 || rows[0].Text != "home-one" || rows[0].Index != 1 {
		t.Errorf("home list = %+v, want exactly one row (#1 home-one)", rows)
	}
}

func TestRunListUnknownProjectErrors(t *testing.T) {
	writeStore(t, task.Store{})

	err := runList(listOptions{Project: "ghost"})
	if err == nil || !strings.Contains(err.Error(), "no such project") {
		t.Errorf("runList(ghost) error = %v, want 'no such project'", err)
	}
}

// --count is the status-bar contract: totals across every project.
func TestRunListCountSpansProjects(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "a", Priority: 1}, {Text: "b", Priority: 2, Done: true}}},
		{Name: "home", Items: []task.Item{{Text: "c", Priority: 2}}},
	}})

	out, err := captureStdout(t, func() error {
		return runList(listOptions{AsCount: true, Project: "work"})
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
	if got.Pending != 2 || got.Completed != 1 || got.Total != 3 {
		t.Errorf("count = %+v, want {2 1 3}", got)
	}
}

func TestProjectListShowsCounts(t *testing.T) {
	writeStore(t, task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "a", Priority: 1}, {Text: "b", Priority: 2, Done: true}}},
		{Name: "empty"},
	}})

	out, err := captureStdout(t, func() error {
		return projectListRun(nil, nil)
	})
	if err != nil {
		t.Fatalf("projectListRun: %v", err)
	}
	if !strings.Contains(out, "work") || !strings.Contains(out, "empty") {
		t.Errorf("project list = %q, want both names", out)
	}
	// work: 1 pending, 1 done, 2 total.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "work") && !strings.Contains(line, "1") {
			t.Errorf("work row %q lacks its pending count", line)
		}
	}
}
