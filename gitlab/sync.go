package gitlab

import (
	"context"
	"errors"
	"net/http"
	"time"

	"todo/task"
)

// LocalState is the part of a linked item the remote side cares about.
type LocalState struct {
	IID          int
	Done         bool
	StateChanged time.Time
}

// LocalItems snapshots every item in the store that mirrors an issue
// of the GitLab project at glPath. The whole store is scanned, not one
// td project: an item keeps its issue when moved between projects and
// must keep updating it. Duplicate IIDs collapse to one entry, a
// completed duplicate winning over a pending one.
func LocalItems(store *task.Store, glPath string) []LocalState {
	var out []LocalState
	seen := map[int]int{}
	for _, p := range store.Projects {
		for _, it := range p.Items {
			if !it.Linked() || it.GitLabProject != glPath {
				continue
			}
			if k, ok := seen[it.GitLabIssue]; ok {
				if it.Done {
					out[k].Done = true
					if it.StateChanged.After(out[k].StateChanged) {
						out[k].StateChanged = it.StateChanged
					}
				}
				continue
			}
			seen[it.GitLabIssue] = len(out)
			out = append(out, LocalState{IID: it.GitLabIssue, Done: it.Done, StateChanged: it.StateChanged})
		}
	}
	return out
}

// Result is the remote half of one sync, ready for Apply.
type Result struct {
	// Open lists the issues still open on the instance.
	Open []Issue
	// Adopted maps an issue IID to the moment it was closed remotely;
	// the local item is older, so it should become done.
	Adopted map[int]time.Time
	// Closed and Reopened count issues whose state td pushed.
	Closed   int
	Reopened int
	// Skipped counts linked issues that no longer exist on the remote.
	Skipped int
}

// SyncRemote performs the remote half of a sync: it reads the open
// issues and pushes the local completion state. A local change newer
// than a remote close wins; an older one yields to the remote (the
// close is reported in Adopted). Remote-side changes are applied one
// at a time, so a failure midway leaves the returned Result usable.
func SyncRemote(ctx context.Context, c *Client, glPath string, local []LocalState) (Result, error) {
	open, err := c.ListOpenIssues(ctx, glPath)
	if err != nil {
		return Result{}, err
	}
	res := Result{Open: open, Adopted: map[int]time.Time{}}

	isOpen := make(map[int]bool, len(open))
	for _, issue := range open {
		isOpen[issue.IID] = true
	}

	for _, it := range local {
		switch {
		case it.Done && isOpen[it.IID]:
			err := c.SetIssueState(ctx, glPath, it.IID, "close")
			if missingIssue(err) {
				res.Skipped++
				continue
			}
			if err != nil {
				return res, err
			}
			res.Closed++

		case !it.Done && !isOpen[it.IID]:
			issue, err := c.Issue(ctx, glPath, it.IID)
			if missingIssue(err) {
				res.Skipped++
				continue
			}
			if err != nil {
				return res, err
			}
			if !issue.Closed() {
				continue
			}
			closedAt := issue.ClosedAtOr()
			if it.StateChanged.After(closedAt) {
				err := c.SetIssueState(ctx, glPath, it.IID, "reopen")
				if missingIssue(err) {
					res.Skipped++
					continue
				}
				if err != nil {
					return res, err
				}
				res.Reopened++
				continue
			}
			res.Adopted[it.IID] = closedAt
		}
	}
	return res, nil
}

// missingIssue reports whether err is a 404 for an issue that was
// deleted on the instance. The item keeps its link but stays local.
func missingIssue(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// Applied counts what merging a Result changed locally.
type Applied struct {
	Imported  int
	Completed int
}

// Changed reports whether the store needs saving.
func (a Applied) Changed() bool { return a.Imported > 0 || a.Completed > 0 }

// Apply merges the remote half into the store: linked items the remote
// already closed become done, and open issues nobody mirrors are added
// to the linked td project as pending items. A missing td project just
// skips the imports.
func Apply(store *task.Store, tdProject, glPath string, res Result) Applied {
	var out Applied

	linked := make(map[int]bool)
	for pi := range store.Projects {
		items := store.Projects[pi].Items
		for k := range items {
			if !items[k].Linked() || items[k].GitLabProject != glPath {
				continue
			}
			linked[items[k].GitLabIssue] = true
			at, ok := res.Adopted[items[k].GitLabIssue]
			if !ok || items[k].Done {
				continue
			}
			items[k].MarkDone(true, at)
			out.Completed++
		}
	}

	target := store.Project(tdProject)
	if target == nil {
		return out
	}
	for _, issue := range res.Open {
		if linked[issue.IID] {
			continue
		}
		target.Items = append(target.Items, task.Item{
			Text:          issue.Title,
			Description:   issue.Description,
			Priority:      2,
			Created:       issue.CreatedAt,
			GitLabIssue:   issue.IID,
			GitLabProject: glPath,
		})
		linked[issue.IID] = true
		out.Imported++
	}
	return out
}
