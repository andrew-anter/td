package gitlab

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"todo/task"
)

func storeWith(projects ...task.Project) *task.Store {
	return &task.Store{Projects: projects}
}

func TestApplyImportsOpenIssues(t *testing.T) {
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	f := newFake(t, "group/gym",
		Issue{IID: 1, Title: "add rack", Description: "two shelves", CreatedAt: created},
		Issue{IID: 2, Title: "buy plates", CreatedAt: created.Add(time.Hour)},
		Issue{IID: 3, Title: "already done", State: "closed"},
	)
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym"})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	applied := Apply(store, "gym", "group/gym", res)
	if applied.Imported != 2 || applied.Completed != 0 || !applied.Changed() {
		t.Fatalf("applied = %+v, want 2 imported", applied)
	}

	items := store.Project("gym").Items
	if len(items) != 2 {
		t.Fatalf("items = %+v, want two", items)
	}
	first := items[0]
	if first.Text != "add rack" || first.Description != "two shelves" || first.Priority != 2 {
		t.Errorf("imported item = %+v", first)
	}
	if first.GitLabIssue != 1 || first.GitLabProject != "group/gym" {
		t.Errorf("imported item lost its link: %+v", first)
	}
	if !first.Created.Equal(created) {
		t.Errorf("imported Created = %v, want %v", first.Created, created)
	}
	if items[0].Done || items[1].Done {
		t.Errorf("imported issues must be pending: %+v", items)
	}
}

func TestSyncPushesLocalCompletionAndReopen(t *testing.T) {
	now := time.Now()
	f := newFake(t, "group/gym",
		Issue{IID: 1, Title: "done locally", State: "opened"},
		Issue{IID: 2, Title: "reopened locally", State: "opened"},
	)
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym", Items: []task.Item{
		{Text: "done locally", Done: true, GitLabIssue: 1, GitLabProject: "group/gym"},
		{Text: "reopened locally", GitLabIssue: 2, GitLabProject: "group/gym", StateChanged: now},
	}})
	// The remote closed issue 2 before the local reopen... simulate by
	// closing it on the fake: the local change is newer, so td reopens.
	closedAt := now.Add(-time.Hour)
	f.issues[1].State = "closed"
	f.issues[1].ClosedAt = &closedAt

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	if res.Closed != 1 || res.Reopened != 1 {
		t.Fatalf("result = %+v, want 1 closed and 1 reopened", res)
	}
	if len(f.events) != 2 || f.events[0] != "close 1" || f.events[1] != "reopen 2" {
		t.Fatalf("events = %v", f.events)
	}

	applied := Apply(store, "gym", "group/gym", res)
	if applied.Changed() {
		t.Errorf("pushing state must not change the local store: %+v", applied)
	}
}

// A remote close newer than the local change wins: this is what makes
// completing things on GitLab's web UI show up in td.
func TestSyncAdoptsNewerRemoteClose(t *testing.T) {
	closedAt := time.Now().Add(-time.Hour)
	f := newFake(t, "group/gym", Issue{IID: 5, Title: "closed upstream", State: "closed", ClosedAt: &closedAt})
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym", Items: []task.Item{
		{Text: "closed upstream", GitLabIssue: 5, GitLabProject: "group/gym", StateChanged: closedAt.Add(-2 * time.Hour)},
	}})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	if len(f.events) != 0 {
		t.Fatalf("remote already closed the issue, yet td pushed %v", f.events)
	}
	if got := res.Adopted[5]; !got.Equal(closedAt) {
		t.Fatalf("adopted = %v, want %v", got, closedAt)
	}

	applied := Apply(store, "gym", "group/gym", res)
	if applied.Completed != 1 {
		t.Fatalf("applied = %+v, want 1 completed", applied)
	}
	it := store.Project("gym").Items[0]
	if !it.Done || !it.Completed.Equal(closedAt) || !it.StateChanged.Equal(closedAt) {
		t.Errorf("adopted item = %+v", it)
	}
}

func TestSyncSkipsDeletedIssue(t *testing.T) {
	// The issue is closed (so it is not in the open list) and then
	// deleted, so the follow-up lookup 404s.
	f := newFake(t, "group/gym", Issue{IID: 7, Title: "gone", State: "closed"})
	f.deleted[7] = true
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym", Items: []task.Item{
		{Text: "gone", GitLabIssue: 7, GitLabProject: "group/gym"},
	}})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("deleted issue must not fail the sync: %v", err)
	}
	if res.Skipped != 1 {
		t.Fatalf("result = %+v, want 1 skipped", res)
	}
	if applied := Apply(store, "gym", "group/gym", res); applied.Changed() {
		t.Errorf("a deleted issue must not change the local item: %+v", applied)
	}
	if store.Project("gym").Items[0].Done {
		t.Errorf("item marked done although the issue is gone")
	}
}

// A close racing an issue deletion must not fail the whole sync.
func TestSyncSkipsCloseOfDeletedIssue(t *testing.T) {
	f := newFake(t, "group/gym", Issue{IID: 3, Title: "done here", State: "opened"})
	f.putGone[3] = true
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym", Items: []task.Item{
		{Text: "done here", Done: true, GitLabIssue: 3, GitLabProject: "group/gym"},
	}})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("deleted issue broke the sync: %v", err)
	}
	if res.Skipped != 1 || res.Closed != 0 {
		t.Errorf("result = %+v, want 1 skipped and nothing closed", res)
	}
}

func TestSyncDoesNotDuplicateLinkedIssue(t *testing.T) {
	f := newFake(t, "group/gym", Issue{IID: 1, Title: "existing", State: "opened"})
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "gym", Items: []task.Item{
		{Text: "existing", GitLabIssue: 1, GitLabProject: "group/gym"},
	}})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	if applied := Apply(store, "gym", "group/gym", res); applied.Imported != 0 {
		t.Fatalf("imported a linked issue again: %+v", applied)
	}
	if n := len(store.Project("gym").Items); n != 1 {
		t.Errorf("items = %d, want 1", n)
	}
}

// An item moved to another td project keeps mirroring its issue, and
// new issues still land in the linked project.
func TestApplyFollowsMovedItems(t *testing.T) {
	closedAt := time.Now().Add(-time.Minute)
	f := newFake(t, "group/gym",
		Issue{IID: 2, Title: "closed upstairs", State: "closed", ClosedAt: &closedAt},
		Issue{IID: 3, Title: "fresh", State: "opened"},
	)
	c, _ := serve(t, f)
	store := storeWith(
		task.Project{Name: "gym"},
		task.Project{Name: "inbox", Items: []task.Item{
			{Text: "moved here", GitLabIssue: 2, GitLabProject: "group/gym", StateChanged: closedAt.Add(-time.Hour)},
			{Text: "unrelated", GitLabIssue: 2, GitLabProject: "other/project"},
		}},
	)

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	applied := Apply(store, "gym", "group/gym", res)
	if applied.Completed != 1 || applied.Imported != 1 {
		t.Fatalf("applied = %+v, want 1 completed and 1 imported", applied)
	}
	moved := store.Project("inbox").Items[0]
	if !moved.Done {
		t.Errorf("moved item did not adopt the remote close")
	}
	other := store.Project("inbox").Items[1]
	if other.Done {
		t.Errorf("item linked to another GitLab project was touched")
	}
	if n := len(store.Project("gym").Items); n != 1 || store.Project("gym").Items[0].Text != "fresh" {
		t.Errorf("new issues must land in the linked project: %+v", store.Project("gym").Items)
	}
}

func TestApplyWithoutLinkedProjectSkipsImports(t *testing.T) {
	f := newFake(t, "group/gym", Issue{IID: 1, Title: "one", State: "opened"})
	c, _ := serve(t, f)
	store := storeWith(task.Project{Name: "inbox"})

	res, err := SyncRemote(context.Background(), c, "group/gym", LocalItems(store, "group/gym"))
	if err != nil {
		t.Fatalf("SyncRemote: %v", err)
	}
	if applied := Apply(store, "deleted-project", "group/gym", res); applied.Changed() {
		t.Fatalf("imported into a missing project: %+v", applied)
	}
}

func TestSyncRemoteFailsOnAuthError(t *testing.T) {
	f := newFake(t, "group/gym")
	c, _ := serve(t, f)
	c.token = "nope"

	_, err := SyncRemote(context.Background(), c, "group/gym", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want a 401 APIError", err)
	}
}
