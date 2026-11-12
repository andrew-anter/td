// Package gitlab bridges td projects to issues on a self-hosted GitLab
// instance. It is entirely optional: nothing here runs unless a URL, a
// token, and at least one project link are configured.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// requestTimeout bounds every call so a hung instance cannot wedge a
// background sync. Syncs issue at most a handful of requests per link.
const requestTimeout = 20 * time.Second

// maxPages caps pagination so a misbehaving instance cannot make one
// sync run forever; 100 pages is 10k open issues.
const maxPages = 100

const userAgent = "td-gitlab-sync"

// Client talks to the GitLab REST API v4.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient validates the connection settings and prepares a client.
// baseURL is the instance root; a subpath deployment
// (https://host/gitlab) works too, and an explicit /api/v4 suffix is
// kept as is.
func NewClient(baseURL, token string) (*Client, error) {
	if err := ValidateURL(baseURL); err != nil {
		return nil, err
	}
	tok := strings.TrimSpace(token)
	if tok == "" {
		return nil, fmt.Errorf("gitlab token is not configured")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(base, "/api/v4") {
		base += "/api/v4"
	}
	return &Client{baseURL: base, token: tok, http: &http.Client{Timeout: requestTimeout}}, nil
}

// Issue is the subset of GitLab's issue payload the sync uses.
type Issue struct {
	IID         int        `json:"iid"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

// Closed reports whether the issue is closed.
func (i Issue) Closed() bool { return i.State == "closed" }

// ClosedAtOr returns the moment the issue was closed, falling back to
// its last update for instances that omit closed_at.
func (i Issue) ClosedAtOr() time.Time {
	if i.ClosedAt != nil {
		return *i.ClosedAt
	}
	return i.UpdatedAt
}

// ListOpenIssues returns every opened issue of project, oldest first,
// following GitLab's pagination headers.
func (c *Client) ListOpenIssues(ctx context.Context, project string) ([]Issue, error) {
	var out []Issue
	page := 1
	for range maxPages {
		q := url.Values{}
		q.Set("state", "opened")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		q.Set("order_by", "created_at")
		q.Set("sort", "asc")

		body, hdr, err := c.do(ctx, http.MethodGet, "projects/"+escape(project)+"/issues?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var batch []Issue
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("gitlab: decoding issues: %w", err)
		}
		out = append(out, batch...)

		next, err := strconv.Atoi(strings.TrimSpace(hdr.Get("X-Next-Page")))
		if err != nil || next <= page {
			return out, nil
		}
		page = next
	}
	return out, nil
}

// Issue fetches one issue; the sync uses it to learn when a linked
// issue that is no longer open was closed.
func (c *Client) Issue(ctx context.Context, project string, iid int) (Issue, error) {
	body, _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("projects/%s/issues/%d", escape(project), iid), nil)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	if err := json.Unmarshal(body, &issue); err != nil {
		return Issue{}, fmt.Errorf("gitlab: decoding issue %d: %w", iid, err)
	}
	return issue, nil
}

// SetIssueState closes or reopens an issue; event is "close" or
// "reopen".
func (c *Client) SetIssueState(ctx context.Context, project string, iid int, event string) error {
	if event != "close" && event != "reopen" {
		return fmt.Errorf("gitlab: unknown state event %q", event)
	}
	path := fmt.Sprintf("projects/%s/issues/%d", escape(project), iid)
	_, _, err := c.do(ctx, http.MethodPut, path, map[string]string{"state_event": event})
	return err
}

// CreateIssue opens a new issue and returns it.
func (c *Client) CreateIssue(ctx context.Context, project, title, description string) (Issue, error) {
	payload := map[string]string{"title": title}
	if description != "" {
		payload["description"] = description
	}
	body, _, err := c.do(ctx, http.MethodPost, "projects/"+escape(project)+"/issues", payload)
	if err != nil {
		return Issue{}, err
	}
	var issue Issue
	if err := json.Unmarshal(body, &issue); err != nil {
		return Issue{}, fmt.Errorf("gitlab: decoding created issue: %w", err)
	}
	return issue, nil
}

// APIError is a non-2xx answer from the instance, with GitLab's own
// message when it sent one.
type APIError struct {
	Method  string
	URL     string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	hint := ""
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		hint = " (check gitlab_token: it needs api scope)"
	case http.StatusNotFound:
		hint = " (check the linked GitLab project path)"
	}
	return fmt.Sprintf("gitlab: %s %s: %d %s%s", e.Method, e.URL, e.Status, e.Message, hint)
}

func (c *Client) do(ctx context.Context, method, path string, payload any) ([]byte, http.Header, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/"+path, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: %s %s: %w", method, req.URL.Redacted(), err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, resp.Header, &APIError{
			Method:  method,
			URL:     req.URL.Redacted(),
			Status:  resp.StatusCode,
			Message: apiMessage(b),
		}
	}
	return b, resp.Header, nil
}

// apiMessage extracts GitLab's "message" field; errors arrive as
// {"message":"401 Unauthorized"} or {"message":{"title":["is too short"]}}.
func apiMessage(body []byte) string {
	var p struct {
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(body, &p) == nil && len(p.Message) > 0 {
		var s string
		if json.Unmarshal(p.Message, &s) == nil {
			return s
		}
		return string(p.Message)
	}
	if msg := strings.TrimSpace(string(body)); msg != "" {
		return msg
	}
	return "unexpected response"
}

// escape encodes a project path ("group/sub/project") for use as a
// single URL path segment.
func escape(project string) string {
	return url.PathEscape(NormalizeProjectPath(project))
}
