/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"todo/task"
)

type rowPair struct{ num, fileIdx int }

func pairs(rows []uiRow) []rowPair {
	out := make([]rowPair, len(rows))
	for k, r := range rows {
		out[k] = rowPair{r.num, r.fileIdx}
	}
	return out
}

func equalPairs(a, b []rowPair) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

// The UI's row numbers must match the CLI's global numbering exactly,
// or numbers shown in the TUI would address the wrong item.
func TestVisibleRowsNumberingMatchesCLI(t *testing.T) {
	items := []task.Item{
		{Text: "milk", Priority: 2, Done: false},        // idx 0
		{Text: "urgent call", Priority: 1, Done: false}, // idx 1
		{Text: "old task A", Priority: 2, Done: true},   // idx 2
		{Text: "write blog", Priority: 3, Done: false},  // idx 3
		{Text: "old task B", Priority: 2, Done: true},   // idx 4
	}

	// Full sorted order: urgent call(1), milk(2), write blog(3),
	// old task A(4), old task B(5).
	cases := []struct {
		name string
		view viewMode
		want []rowPair
	}{
		{"pending", viewPending, []rowPair{{1, 1}, {2, 0}, {3, 3}}},
		{"done", viewDone, []rowPair{{4, 2}, {5, 4}}},
		{"all", viewAll, []rowPair{{1, 1}, {2, 0}, {3, 3}, {4, 2}, {5, 4}}},
	}
	for _, tc := range cases {
		got := pairs(visibleRows(items, tc.view))
		if !equalPairs(got, tc.want) {
			t.Errorf("%s: rows = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// A number shown in the TUI for any view must target the same item as
// the CLI's sortedOrderIndices — the invariant every view must keep.
func TestVisibleRowFileIdxAgreesWithSortedOrderIndices(t *testing.T) {
	items := []task.Item{
		{Text: "a", Priority: 3, Done: false},
		{Text: "b", Priority: 1, Done: true},
		{Text: "c", Priority: 2, Done: true},
		{Text: "d", Priority: 1, Done: false},
		{Text: "e", Priority: 2, Done: false},
	}
	order := sortedOrderIndices(items)
	for _, view := range []viewMode{viewPending, viewAll, viewDone} {
		for _, r := range visibleRows(items, view) {
			if order[r.num-1] != r.fileIdx {
				t.Errorf("view %v: row num %d has fileIdx %d, CLI expects %d",
					view, r.num, r.fileIdx, order[r.num-1])
			}
		}
	}
}

func TestToggleAndDeleteItem(t *testing.T) {
	items := []task.Item{
		{Text: "keep", Priority: 2, Done: false},
		{Text: "flip", Priority: 2, Done: false},
	}
	toggleItem(items, 1)
	if !items[1].Done || items[0].Done {
		t.Errorf("toggleItem hit the wrong element: %+v", items)
	}
	if items[1].Completed.IsZero() {
		t.Errorf("toggleItem did not stamp Completed: %+v", items[1])
	}
	toggleItem(items, 1)
	if items[1].Done || !items[1].Completed.IsZero() {
		t.Errorf("reopening did not clear Done/Completed: %+v", items[1])
	}
	items = deleteItem(items, 0)
	if len(items) != 1 || items[0].Text != "flip" {
		t.Errorf("deleteItem removed the wrong element: %+v", items)
	}
}

// Round-tripping the model's snapshot through SaveStore/ReadStore is
// what persist() does on every mutation.
func TestUIPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{
		Name:  task.DefaultProject,
		Items: []task.Item{{Text: "one", Priority: 2, Done: false}},
	}}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = append(m.items, task.Item{Text: "two", Priority: 1, Done: true})
	m.persist("added")
	if m.status != "added" {
		t.Fatalf("persist set status %q, want %q", m.status, "added")
	}

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	p := got.Project(task.DefaultProject)
	if p == nil || len(p.Items) != 2 || p.Items[1].Text != "two" || !p.Items[1].Done {
		t.Fatalf("saved file = %+v", got)
	}
}

// The project switcher must cycle through the store's namespaces in
// order, wrapping both ways, and carry each project's own items.
func TestSwitchProjectCycles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "i", Priority: 2}}},
		{Name: "work", Items: []task.Item{{Text: "w", Priority: 2}}},
	}}
	m := newUIModel(path, store, "inbox")

	m.switchProject(1)
	if m.project != "work" || len(m.items) != 1 || m.items[0].Text != "w" {
		t.Fatalf("after next: project=%q items=%+v", m.project, m.items)
	}
	m.switchProject(1)
	if m.project != "inbox" {
		t.Errorf("forward wrap-around failed: project=%q", m.project)
	}
	m.switchProject(-1)
	if m.project != "work" {
		t.Errorf("backward wrap-around failed: project=%q", m.project)
	}
}

// persist() must write the active project's items and leave every
// other project in the store untouched.
func TestPersistKeepsOtherProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "keep me", Priority: 2}}},
	}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = append(m.items, task.Item{Text: "new inbox item", Priority: 2})
	m.persist("added")

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if w := got.Project("work"); w == nil || len(w.Items) != 1 || w.Items[0].Text != "keep me" {
		t.Errorf("work project clobbered: %+v", got)
	}
	in := got.Project(task.DefaultProject)
	if in == nil || len(in.Items) != 1 || in.Items[0].Text != "new inbox item" {
		t.Errorf("inbox persist wrong: %+v", in)
	}
}

// createProjectInPlace validates, adds, and persists a new namespace
// without leaving the projects screen or switching the active project;
// duplicates and invalid names are refused.
func TestCreateProjectInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	m := newUIModel(path, task.Store{}, task.DefaultProject)

	if err := m.createProjectInPlace("gym"); err != nil {
		t.Fatalf("createProjectInPlace: %v", err)
	}
	if m.project != task.DefaultProject {
		t.Errorf("create must not switch the active project: %q", m.project)
	}
	if m.status != `created project "gym"` || m.pickerIdx != 1 {
		t.Errorf("after create: status=%q pickerIdx=%d", m.status, m.pickerIdx)
	}
	if err := m.createProjectInPlace("gym"); err == nil {
		t.Errorf("expected duplicate project error")
	}
	if err := m.createProjectInPlace("two words"); err != nil {
		t.Errorf("names with spaces should be accepted: %v", err)
	}
	if err := m.createProjectInPlace("tab\there"); err == nil {
		t.Errorf("expected control character error")
	}

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	names := got.ProjectNames()
	if len(names) != 3 || names[0] != task.DefaultProject || names[1] != "gym" || names[2] != "two words" {
		t.Errorf("persisted projects = %v, want [inbox gym two words]", names)
	}
}

// renameProject keeps items and follows the view when the active
// project is renamed; deleteProject refuses non-empty projects and
// re-anchors the view when the active one goes.
func TestRenameAndDeleteProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "work", Items: []task.Item{{Text: "keep", Priority: 2}}},
		{Name: "empty"},
	}}
	m := newUIModel(path, store, "work")

	m.renameTarget = "work"
	if err := m.renameProject("job"); err != nil {
		t.Fatalf("renameProject: %v", err)
	}
	if m.project != "job" {
		t.Errorf("active project did not follow rename: %q", m.project)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if got.Project("work") != nil {
		t.Errorf("old name persisted")
	}
	if p := got.Project("job"); p == nil || len(p.Items) != 1 || p.Items[0].Text != "keep" {
		t.Errorf("rename lost items: %+v", p)
	}

	// d on a non-empty project refuses.
	m.deleteProject("job")
	if got2, _ := task.ReadStore(path); got2.Project("job") == nil {
		t.Errorf("non-empty project was deleted without force")
	}
	if !strings.Contains(m.status, "has 1 item") {
		t.Errorf("status = %q, want refusal with item count", m.status)
	}

	// d on the empty active project removes it and re-anchors.
	m.setProject("empty")
	m.deleteProject("empty")
	if m.project != "job" {
		t.Errorf("after deleting active project, view = %q, want job", m.project)
	}
	got3, _ := task.ReadStore(path)
	if got3.Project("empty") != nil {
		t.Errorf("empty project persisted after delete")
	}
}

// setProject must swap both the name and the item slice the views and
// mutations operate on.
func TestSetProjectSwapsItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "a", Items: []task.Item{{Text: "in a", Priority: 2}}},
		{Name: "b", Items: []task.Item{{Text: "in b1", Priority: 2}, {Text: "in b2", Priority: 2}}},
	}}
	m := newUIModel(path, store, "a")
	if len(m.items) != 1 {
		t.Fatalf("model items = %+v", m.items)
	}
	m.setProject("b")
	if m.project != "b" || len(m.items) != 2 || m.items[0].Text != "in b1" {
		t.Errorf("after setProject(b): project=%q items=%+v", m.project, m.items)
	}
	m.setProject("fresh")
	if m.project != "fresh" || len(m.items) != 0 {
		t.Errorf("setProject on a missing name should give an empty namespace: %+v", m.items)
	}
}

// moveSelectedTo relocates the cursor row into another project,
// preserves its state, persists immediately, and moveTargets excludes
// the current namespace.
func TestMoveSelectedTo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "a", Items: []task.Item{{Text: "y", Priority: 2}, {Text: "x", Priority: 1}}},
		{Name: "b"},
	}}
	m := newUIModel(path, store, "a")

	if names := m.moveTargets(); len(names) != 1 || names[0] != "b" {
		t.Fatalf("moveTargets = %v, want [b]", names)
	}

	// Cursor starts on row 1: "x" (P1 sorts first).
	m.moveSelectedTo("b")
	if m.status != `moved "x" to "b"` {
		t.Fatalf("status = %q", m.status)
	}
	if len(m.items) != 1 || m.items[0].Text != "y" {
		t.Errorf("source items = %+v, want only y", m.items)
	}

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	b := got.Project("b")
	if b == nil || len(b.Items) != 1 || b.Items[0].Text != "x" || b.Items[0].Priority != 1 {
		t.Errorf("target project = %+v, want x (P1)", b)
	}
}

// The projects screen must fill the same vertical space as the task
// list, so the context help sits at the bottom of the screen in both.
func TestProjectsScreenPadsToBottom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "milk", Priority: 2}}},
		{Name: "td"},
	}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	m.mode = modeProjects
	projectsLines := strings.Count(m.View().Content, "\n")

	m.mode = modeNormal
	listLines := strings.Count(m.View().Content, "\n")

	if projectsLines < 25 {
		t.Errorf("projects screen only %d lines tall on a 30-line terminal — footer not pinned", projectsLines)
	}
	if projectsLines != listLines {
		t.Errorf("projects screen %d lines, tasks screen %d — footers misaligned", projectsLines, listLines)
	}
}

// After deleting the top pending item, the survivor is renumbered to
// 1 and stays addressable in the same view.
func TestVisibleRowsAfterDeleteRenumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{
		Name: task.DefaultProject,
		Items: []task.Item{
			{Text: "a", Priority: 1, Done: false},
			{Text: "b", Priority: 2, Done: false},
		},
	}}}
	m := newUIModel(path, store, task.DefaultProject)
	m.items = deleteItem(m.items, m.visibleRows()[0].fileIdx)
	m.refresh("deleted")

	got := pairs(m.visibleRows())
	want := []rowPair{{1, 0}} // "b" now first in file and display order
	if !equalPairs(got, want) {
		t.Fatalf("rows after delete = %+v, want %+v", got, want)
	}
	if m.items[0].Text != "b" {
		t.Errorf("fileIdx 0 should now be %q, got %q", "b", m.items[0].Text)
	}
}

// Long task text must stay inside the terminal: the edit block wraps
// instead of sliding a one-line viewport whose caret ends up past the
// right screen edge.
func TestEditBlockStaysWithinTerminalWidth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "milk"}}},
	}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	m.help.SetWidth(m.width)

	m.mode = modeAdd
	m.input.SetValue("buy a really long amount of groceries for the entire week including milk eggs bread cheese and vegetables")
	m.input.Enter()
	m.input.Focus()

	content := m.View().Content
	lines := strings.Split(content, "\n")
	if len(lines) < 5 {
		t.Fatalf("content is %d lines, want the edit block to push past one", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d cells wide, exceeds the %d-cell terminal — caret would be off-screen: %q", i, w, m.width, line)
		}
	}
	if !strings.Contains(content, "vegetables") {
		t.Errorf("tail of the wrapped text missing from the view")
	}
}

// A 300-char task must not make list rows or the status line wider
// than the terminal; rows truncate, the edit view shows the full text.
func TestLongTaskRowsAndStatusFitWidth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	long := strings.Repeat("word ", 60)
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: long}}},
	}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	m.help.SetWidth(m.width)

	m.status = fmt.Sprintf("added %q", long)
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d cells, want ≤ %d: %q", i, w, m.width, line)
		}
	}
	if !strings.Contains(m.View().Content, "…") {
		t.Errorf("no ellipsis — row or status not truncated")
	}
}

func pressKeys(m *uiModel, keys []string) {
	for _, k := range keys {
		updated, _ := m.handleKey(keyMsg(k))
		*m = updated.(uiModel)
	}
}

func TestUIDescribeKeySetsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "fix the sink", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	pressKeys(&m, []string{"D", "k", "e", "r", "k", "e", "d", " ", "t", "i", "g", "h", "t", "enter"})

	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after commit", m.mode)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if d := got.Projects[0].Items[0].Description; d != "kerked tight" {
		t.Fatalf("persisted Description = %q, want %q", d, "kerked tight")
	}
}

func TestUIDescribeCancelKeepsText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "fix the sink", Priority: 2, Description: "original"},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	m.persist("seeded")

	pressKeys(&m, []string{"D", "x", "y", "z", "esc", "esc"})

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if d := got.Projects[0].Items[0].Description; d != "original" {
		t.Fatalf("cancel changed Description to %q", d)
	}
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after cancel", m.mode)
	}
}

func TestUIPreviewShowsSelectedDescription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "fix the sink", Priority: 2, Description: "kerl valve stripped"},
		{Text: "no desc", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	content := m.View().Content
	if !strings.Contains(content, "↳ kerl valve stripped") {
		t.Fatalf("preview line missing from view:\n%s", content)
	}
	withPreview := strings.Count(content, "\n")

	pressKeys(&m, []string{"j"})
	after := m.View().Content
	if strings.Contains(after, "↳") {
		t.Fatalf("preview shown for item without description:\n%s", after)
	}
	if strings.Count(after, "\n") != withPreview {
		t.Errorf("total height changed with the preview — footer no longer pinned")
	}
}

func TestUIEditorFinishAppliesTitleAndDesc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "fix the sink", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")

	f := filepath.Join(t.TempDir(), "buffer.md")
	if err := os.WriteFile(f, []byte("new title\n\npara one\n\npara two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(editTaskFinishedMsg{path: f, fileIdx: 0})
	m = updated.(uiModel)

	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	it := got.Projects[0].Items[0]
	if it.Text != "new title" || it.Description != "para one\n\npara two" {
		t.Fatalf("item after editor = %+v", it)
	}
	if _, statErr := os.Stat(f); !os.IsNotExist(statErr) {
		t.Errorf("editor temp file was not cleaned up")
	}
}

func TestUIEditorCancelKeepsItem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "keep me", Priority: 2, Description: "intact"},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.persist("seeded")

	f := filepath.Join(t.TempDir(), "buffer.md")
	if err := os.WriteFile(f, []byte("half edited junk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Update(editTaskFinishedMsg{err: errors.New("editor exited nonzero"), path: f, fileIdx: 0})

	got, _ := task.ReadStore(path)
	it := got.Projects[0].Items[0]
	if it.Text != "keep me" || it.Description != "intact" {
		t.Fatalf("failed editor run mutated the item: %+v", it)
	}
}

func TestUIPreviewFlattensParagraphBreaks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "t", Priority: 2, Description: "para one\npara two"},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	content := m.View().Content
	if !strings.Contains(content, "↳ para one para two") {
		t.Fatalf("preview did not flatten breaks:\n%s", content)
	}
	if strings.Contains(content, "para one\npara two") {
		t.Errorf("preview leaked a hard break into the layout")
	}
}

// D flattens newlines when loading; committing without edits must not
// rewrite the stored paragraph bytes.
func TestUIDescribeUntouchedKeepsParagraphs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "t", Priority: 2, Description: "one\n\ntwo"},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()
	m.persist("seeded")

	pressKeys(&m, []string{"D", "enter"})

	got, _ := task.ReadStore(path)
	if d := got.Projects[0].Items[0].Description; d != "one\n\ntwo" {
		t.Fatalf("untouched D commit flattened Description to %q", d)
	}
}

func TestUIEditorMissingReportsStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "t", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	t.Setenv("EDITOR", "no-such-editor-xyz")
	updated, cmd := m.handleKey(keyMsg("E"))
	m = updated.(uiModel)

	if cmd != nil {
		t.Errorf("spawned a process for a missing editor")
	}
	if !strings.Contains(m.status, "no-such-editor-xyz") || !strings.Contains(m.status, "set $EDITOR") {
		t.Fatalf("status = %q, want a clear missing-editor hint", m.status)
	}
}

func TestEnterOpensDetailXCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	pressKeys(&m, []string{"enter"})
	if m.mode != modeDetail {
		t.Fatalf("enter opened mode %v, want detail", m.mode)
	}

	// x completes from the detail screen and drops back to the list.
	pressKeys(&m, []string{"x"})
	if m.mode != modeNormal {
		t.Errorf("x left mode %v, want normal", m.mode)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if !got.Projects[0].Items[0].Done {
		t.Errorf("x did not complete the item: %+v", got)
	}

	pressKeys(&m, []string{"enter", "esc"})
	if m.mode != modeNormal {
		t.Errorf("esc left mode %v, want normal", m.mode)
	}

	// q closes the detail screen instead of quitting the app. The
	// completed row only shows again on the all view.
	pressKeys(&m, []string{"tab"})
	updated, _ := m.handleKey(keyMsg("enter"))
	m = updated.(uiModel)
	updated, cmd := m.handleKey(keyMsg("q"))
	m = updated.(uiModel)
	if m.mode != modeNormal {
		t.Errorf("q left mode %v, want normal", m.mode)
	}
	if cmd != nil {
		t.Errorf("q in detail returned a command, want back-not-quit")
	}

	// Empty list: enter opens nothing.
	e := newUIModel(filepath.Join(t.TempDir(), "empty.json"), task.Store{}, "inbox")
	updated, _ = e.handleKey(keyMsg("enter"))
	if updated.(uiModel).mode != modeNormal {
		t.Errorf("enter on empty list opened a detail screen")
	}
}

func TestDetailViewShowsFullTextAndMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	long := strings.Repeat("alpha bravo charlie ", 8) + "ZULU-ENDING"
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: long, Description: "first paragraph tail-one\n\nsecond paragraph tail-two", Priority: 1},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 40
	(&m).resizeList()
	pressKeys(&m, []string{"enter"})

	content := m.View().Content
	body := m.detailView()
	if !strings.Contains(body, "ZULU-ENDING") {
		t.Errorf("detail body did not show the full text tail")
	}
	if strings.Contains(body, "…") {
		t.Errorf("detail body ellipsized long text")
	}
	if !strings.Contains(content, "second paragraph tail-two") {
		t.Errorf("detail view lost the description's second paragraph")
	}
	for _, want := range []string{"high", "td complete 1 -P inbox", "pending"} {
		if !strings.Contains(content, want) {
			t.Errorf("detail view missing %q", want)
		}
	}
}

// An applied filter narrows the rows but must not become a modal input:
// task verbs still act on the visible selection.
func TestAppliedFilterKeepsTaskVerbsLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
		{Text: "bread", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	m.list.SetFilterText("milk")
	if m.list.FilterState() != list.FilterApplied {
		t.Fatalf("FilterState = %v, want FilterApplied", m.list.FilterState())
	}

	pressKeys(&m, []string{"x"})
	if !m.items[0].Done {
		t.Fatalf("x under an applied filter did not complete the selected task")
	}
	if m.items[1].Done {
		t.Errorf("x toggled a task outside the filter")
	}
}

// esc clears an applied filter; it must not fall through to the app's
// quit binding.
func TestEscClearsAppliedFilterInsteadOfQuitting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	m.list.SetFilterText("milk")
	updated, cmd := m.handleKey(keyMsg("esc"))
	m = updated.(uiModel)

	if m.list.FilterState() != list.Unfiltered {
		t.Fatalf("esc left FilterState = %v, want Unfiltered", m.list.FilterState())
	}
	if cmd != nil {
		t.Errorf("esc under an applied filter returned a command, want a cleared filter")
	}
}

// d must not destroy a task outright: the prompt names the target, n
// and esc cancel, and only y commits the delete.
func TestDeleteTaskWaitsForConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "milk", Priority: 2},
		{Text: "bread", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	pressKeys(&m, []string{"d"})
	if m.mode != modeConfirm || m.confirm == nil {
		t.Fatalf("d opened mode %v, want a confirmation prompt", m.mode)
	}
	if len(m.items) != 2 {
		t.Fatalf("d deleted before confirmation: %d items left", len(m.items))
	}
	content := m.View().Content
	if !strings.Contains(content, `delete "milk"?`) {
		t.Errorf("prompt does not name the task:\n%s", content)
	}
	if !strings.Contains(content, "confirm delete") || !strings.Contains(content, "cancel") {
		t.Errorf("footer does not offer confirm/cancel:\n%s", content)
	}

	// A stray verb while the prompt is open changes nothing.
	pressKeys(&m, []string{"x"})
	if len(m.items) != 2 || m.mode != modeConfirm {
		t.Fatalf("key during confirmation mutated state: mode=%v items=%d", m.mode, len(m.items))
	}

	pressKeys(&m, []string{"n"})
	if m.mode != modeNormal || len(m.items) != 2 {
		t.Fatalf("n did not cancel: mode=%v items=%d", m.mode, len(m.items))
	}
	if m.status != "delete cancelled" {
		t.Errorf("status = %q, want cancel message", m.status)
	}

	pressKeys(&m, []string{"d", "esc"})
	if m.mode != modeNormal || len(m.items) != 2 {
		t.Fatalf("esc did not cancel: mode=%v items=%d", m.mode, len(m.items))
	}

	pressKeys(&m, []string{"d", "y"})
	if m.mode != modeNormal || len(m.items) != 1 || m.items[0].Text != "bread" {
		t.Fatalf("y did not delete the target: mode=%v items=%+v", m.mode, m.items)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if p := got.Project("inbox"); p == nil || len(p.Items) != 1 || p.Items[0].Text != "bread" {
		t.Errorf("deletion did not persist: %+v", got)
	}
}

// Project deletion follows the same rule: d asks, and the projects
// screen stays behind the prompt.
func TestDeleteProjectWaitsForConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "inbox", Items: []task.Item{{Text: "keep", Priority: 2}}},
		{Name: "scratch"},
	}}
	m := newUIModel(path, store, "inbox")
	m.width, m.height = 80, 30
	(&m).resizeList()

	pressKeys(&m, []string{"p", "j", "d"})
	if m.mode != modeConfirm || m.confirm == nil {
		t.Fatalf("d opened mode %v, want a confirmation prompt", m.mode)
	}
	if m.store.Project("scratch") == nil {
		t.Fatal("project deleted before confirmation")
	}
	content := m.View().Content
	if !strings.Contains(content, `delete project "scratch"?`) {
		t.Errorf("prompt does not name the project:\n%s", content)
	}
	if !strings.Contains(content, "scratch") || !strings.Contains(content, "pending") {
		t.Errorf("projects screen hidden behind the prompt:\n%s", content)
	}

	pressKeys(&m, []string{"n"})
	if m.mode != modeProjects || m.store.Project("scratch") == nil {
		t.Fatalf("n left mode=%v, project present=%v", m.mode, m.store.Project("scratch") != nil)
	}

	pressKeys(&m, []string{"d", "y"})
	if m.mode != modeProjects || m.store.Project("scratch") != nil {
		t.Fatalf("y left mode=%v, project present=%v", m.mode, m.store.Project("scratch") != nil)
	}
	got, err := task.ReadStore(path)
	if err != nil {
		t.Fatalf("ReadStore: %v", err)
	}
	if got.Project("scratch") != nil {
		t.Errorf("project deletion did not persist: %+v", got)
	}
	if got.Project("inbox") == nil {
		t.Errorf("confirmation deleted the wrong project: %+v", got)
	}
}
