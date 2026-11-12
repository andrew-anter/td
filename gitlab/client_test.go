package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeGitLab serves the subset of the REST API the client uses and
// records the calls it received.
type fakeGitLab struct {
	t        *testing.T
	project  string
	token    string
	issues   []Issue
	nextIID  int
	events   []string // "close 1"
	requests []string // "GET /api/v4/..."
	pageSize int      // 0 serves everything in one page
	deleted  map[int]bool
	putGone  map[int]bool
	closedAt time.Time
}

func newFake(t *testing.T, project string, issues ...Issue) *fakeGitLab {
	f := &fakeGitLab{t: t, project: project, token: "token", issues: issues, deleted: map[int]bool{}, putGone: map[int]bool{}}
	for _, is := range issues {
		if is.IID > f.nextIID {
			f.nextIID = is.IID
		}
	}
	return f
}

func (f *fakeGitLab) index(iid int) int {
	for k := range f.issues {
		if f.issues[k].IID == iid {
			return k
		}
	}
	return -1
}

func (f *fakeGitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != f.token {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"401 Unauthorized"}`)
		return
	}
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())

	prefix := "/api/v4/projects/" + f.project + "/issues"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Project Not Found"}`)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	w.Header().Set("Content-Type", "application/json")

	switch {
	case rest == "" && r.Method == http.MethodGet:
		var open []Issue
		for _, is := range f.issues {
			if !is.Closed() {
				open = append(open, is)
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size := f.pageSize
		if size <= 0 || size > len(open) {
			size = max(len(open), 1)
		}
		start := min((page-1)*size, len(open))
		end := min(start+size, len(open))
		if end < len(open) {
			w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
		}
		_ = json.NewEncoder(w).Encode(open[start:end])

	case rest == "" && r.Method == http.MethodPost:
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			f.t.Errorf("create issue body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.nextIID++
		issue := Issue{
			IID:         f.nextIID,
			Title:       payload["title"],
			Description: payload["description"],
			State:       "opened",
			CreatedAt:   time.Now(),
		}
		f.issues = append(f.issues, issue)
		_ = json.NewEncoder(w).Encode(issue)

	case strings.HasPrefix(rest, "/"):
		iid, err := strconv.Atoi(strings.TrimPrefix(rest, "/"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.deleted[iid] {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not found"}`)
			return
		}
		k := f.index(iid)
		if k < 0 {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not found"}`)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(f.issues[k])
		case http.MethodPut:
			if f.putGone[iid] {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Not found"}`)
				return
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				f.t.Errorf("state body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.events = append(f.events, payload["state_event"]+" "+strconv.Itoa(iid))
			switch payload["state_event"] {
			case "close":
				closed := f.closedAt
				if closed.IsZero() {
					closed = time.Now()
				}
				f.issues[k].State = "closed"
				f.issues[k].ClosedAt = &closed
			case "reopen":
				f.issues[k].State = "opened"
				f.issues[k].ClosedAt = nil
			}
			_ = json.NewEncoder(w).Encode(f.issues[k])
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func serve(t *testing.T, f *fakeGitLab) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "token")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func TestNewClientBuildsAPIBase(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://git.example.com", "https://git.example.com/api/v4"},
		{"https://git.example.com/", "https://git.example.com/api/v4"},
		{"https://git.example.com/gitlab", "https://git.example.com/gitlab/api/v4"},
		{"https://git.example.com/api/v4", "https://git.example.com/api/v4"},
	}
	for _, c := range cases {
		got, err := NewClient(c.url, "tok")
		if err != nil {
			t.Fatalf("NewClient(%q): %v", c.url, err)
		}
		if got.baseURL != c.want {
			t.Errorf("NewClient(%q).baseURL = %q, want %q", c.url, got.baseURL, c.want)
		}
	}

	for _, bad := range []struct{ url, token string }{
		{"", "tok"},
		{"git.example.com", "tok"},
		{"ftp://git.example.com", "tok"},
		{"https://git.example.com", ""},
	} {
		if _, err := NewClient(bad.url, bad.token); err == nil {
			t.Errorf("NewClient(%q, %q) succeeded, want error", bad.url, bad.token)
		}
	}
}

// GitLab identifies a project by its URL-escaped path, so a nested
// group must arrive as one path segment.
func TestListOpenIssuesEscapesProjectPathAndPaginates(t *testing.T) {
	f := newFake(t, "group/sub/proj",
		Issue{IID: 1, Title: "one", State: "opened"},
		Issue{IID: 2, Title: "two", State: "opened"},
		Issue{IID: 3, Title: "three", State: "opened"},
		Issue{IID: 9, Title: "done", State: "closed"},
	)
	f.pageSize = 2
	c, _ := serve(t, f)

	got, err := c.ListOpenIssues(context.Background(), "group/sub/proj")
	if err != nil {
		t.Fatalf("ListOpenIssues: %v", err)
	}
	if len(got) != 3 || got[0].IID != 1 || got[2].IID != 3 {
		t.Fatalf("issues = %+v, want 1,2,3", got)
	}
	if !strings.Contains(f.requests[0], "projects/group%2Fsub%2Fproj/issues") {
		t.Errorf("project path not escaped as one segment: %s", f.requests[0])
	}
	if len(f.requests) < 2 {
		t.Errorf("pagination not followed: %v", f.requests)
	}
}

func TestClientSurfacesAPIMessage(t *testing.T) {
	f := newFake(t, "group/proj")
	c, srv := serve(t, f)
	c.token = "wrong"

	_, err := c.ListOpenIssues(context.Background(), "group/proj")
	if err == nil {
		t.Fatal("expected an authentication error")
	}
	msg := err.Error()
	for _, want := range []string{"401", "401 Unauthorized", "check gitlab_token", srv.URL} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestSetIssueStateAndCreateIssue(t *testing.T) {
	f := newFake(t, "group/proj", Issue{IID: 7, Title: "fix", State: "opened"})
	c, _ := serve(t, f)
	ctx := context.Background()

	if err := c.SetIssueState(ctx, "group/proj", 7, "close"); err != nil {
		t.Fatalf("SetIssueState: %v", err)
	}
	if len(f.events) != 1 || f.events[0] != "close 7" {
		t.Fatalf("events = %v, want [close 7]", f.events)
	}
	if !f.issues[0].Closed() {
		t.Errorf("issue not closed on the fake")
	}
	if err := c.SetIssueState(ctx, "group/proj", 7, "explode"); err == nil {
		t.Errorf("invalid state event accepted")
	}

	created, err := c.CreateIssue(ctx, "group/proj", "new title", "details")
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if created.IID != 8 || created.Title != "new title" || created.Description != "details" {
		t.Errorf("created = %+v", created)
	}
}

func TestIssueNotFound(t *testing.T) {
	f := newFake(t, "group/proj", Issue{IID: 4, Title: "gone", State: "opened"})
	f.deleted[4] = true
	c, _ := serve(t, f)

	_, err := c.Issue(context.Background(), "group/proj", 4)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want a 404 APIError", err)
	}
}
