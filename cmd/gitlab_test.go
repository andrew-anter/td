/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	"github.com/spf13/viper"

	"todo/gitlab"
	"todo/task"
)

func newGitlabModel(t *testing.T, cfgPath string) (*uiModel, string) {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{
		{Name: "gym"},
		{Name: "inbox", Items: []task.Item{{Text: "milk", Priority: 2}}},
	}}
	m := newUIModel(dataPath, store, "gym")
	m.configPath = cfgPath
	m.gitlab = gitlab.LoadFileSettings(cfgPath)
	m.gitlabEnv = gitlab.EnvOverrides()
	m.width, m.height = 100, 30
	(&m).resizeList()
	return &m, dataPath
}

// The page must configure everything a sync needs — instance URL,
// token, and links — and write it to the td config file immediately.
func TestGitLabPageConfiguresURLTokenAndLinks(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)

	pressKeys(m, []string{"g"})
	if m.mode != modeGitLab {
		t.Fatalf("g opened mode %v, want the GitLab page", m.mode)
	}
	content := m.View().Content
	for _, want := range []string{"gitlab", "url", "token", "(not set)", "td works without this"} {
		if !strings.Contains(content, want) {
			t.Errorf("page missing %q:\n%s", want, content)
		}
	}

	pressKeys(m, []string{"enter"})
	if m.mode != modeGitLabURL {
		t.Fatalf("enter on the url row opened mode %v", m.mode)
	}
	m.input.SetValue("https://git.example.com")
	pressKeys(m, []string{"enter"})
	if m.mode != modeGitLab {
		t.Fatalf("committing the url left mode %v", m.mode)
	}

	pressKeys(m, []string{"j", "enter"})
	if m.mode != modeGitLabToken {
		t.Fatalf("enter on the token row opened mode %v", m.mode)
	}
	m.input.SetValue("glpat-secret-1234")
	pressKeys(m, []string{"enter"})

	pressKeys(m, []string{"a"})
	if m.mode != modeGitLabTd {
		t.Fatalf("a opened mode %v, want the td project field", m.mode)
	}
	m.input.SetValue("gym")
	pressKeys(m, []string{"enter"})
	if m.mode != modeGitLabRef {
		t.Fatalf("committing the project left mode %v, want the GitLab path field", m.mode)
	}
	m.input.SetValue("group/gym")
	pressKeys(m, []string{"enter"})
	if m.mode != modeGitLab {
		t.Fatalf("committing the path left mode %v", m.mode)
	}
	if m.gitlabIdx != 2 {
		t.Errorf("cursor = %d, want the new link row", m.gitlabIdx)
	}

	got := gitlab.LoadFileSettings(cfg)
	if got.URL != "https://git.example.com" || got.Token != "glpat-secret-1234" {
		t.Errorf("saved settings = %+v", got)
	}
	if got.Links["gym"] != "group/gym" {
		t.Errorf("saved links = %v", got.Links)
	}
	if content := m.View().Content; !strings.Contains(content, "→ group/gym") {
		t.Errorf("page does not show the new link:\n%s", content)
	}

	pressKeys(m, []string{"d"})
	if _, ok := m.gitlab.Links["gym"]; ok {
		t.Fatalf("d did not remove the link: %v", m.gitlab.Links)
	}
	if got := gitlab.LoadFileSettings(cfg); len(got.Links) != 0 {
		t.Errorf("link still in the config: %v", got.Links)
	}
}

func TestGitLabPageRefusesUnknownTdProject(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)

	pressKeys(m, []string{"g", "a"})
	m.input.SetValue("nope")
	pressKeys(m, []string{"enter"})

	if m.mode != modeGitLab {
		t.Fatalf("mode = %v, want the page", m.mode)
	}
	if !strings.Contains(m.status, `no project "nope"`) {
		t.Errorf("status = %q, want a missing-project message", m.status)
	}
	if len(m.gitlab.Links) != 0 {
		t.Errorf("link saved for a missing project: %v", m.gitlab.Links)
	}
}

func TestGitLabPageRefusesDuplicateGitLabProject(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)
	m.gitlab.Links = map[string]string{"inbox": "group/shared"}
	m.gitlabIdx = 2
	pressKeys(m, []string{"g"})

	pressKeys(m, []string{"a"})
	m.input.SetValue("gym")
	pressKeys(m, []string{"enter"})
	m.input.SetValue("group/shared")
	pressKeys(m, []string{"enter"})

	if _, ok := m.gitlab.Links["gym"]; ok {
		t.Fatalf("duplicate GitLab project was linked: %v", m.gitlab.Links)
	}
	if !strings.Contains(m.status, `already linked to "inbox"`) {
		t.Errorf("status = %q, want a duplicate-link message", m.status)
	}
}

func TestGitLabPageCancelReturnsToPage(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)

	pressKeys(m, []string{"g", "a"})
	if m.mode != modeGitLabTd {
		t.Fatalf("mode = %v", m.mode)
	}
	pressKeys(m, []string{"esc", "esc"})
	if m.mode != modeGitLab {
		t.Fatalf("cancel left mode %v, want the page", m.mode)
	}
	pressKeys(m, []string{"esc"})
	if m.mode != modeNormal {
		t.Fatalf("esc on the page left mode %v, want the task list", m.mode)
	}
}

// A local-only setup must never build a sync command, and a link only
// syncs the project it belongs to.
func TestStartupSyncRequiresConfiguredLink(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)
	if cmd := m.startupSyncCmd(); cmd != nil {
		t.Fatalf("unconfigured td built a sync command")
	}

	m.gitlab = gitlab.Settings{URL: "https://git.example.com", Token: "tok", Links: map[string]string{"inbox": "group/inbox"}}
	if cmd := m.startupSyncCmd(); cmd != nil {
		t.Fatalf("project without a link built a sync command")
	}
	m.gitlab.Links["gym"] = "group/gym"
	if cmd := m.startupSyncCmd(); cmd == nil {
		t.Fatalf("linked, configured project did not schedule a sync")
	}

	m.gitlab.Token = ""
	if cmd := m.startupSyncCmd(); cmd != nil {
		t.Fatalf("missing token built a sync command")
	}
}

func TestGitlabSyncedMsgAdoptsStore(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)
	m.list.SetFilterText("from")
	if m.list.FilterState() != list.FilterApplied {
		t.Fatalf("filter state = %v, want applied", m.list.FilterState())
	}

	imported := task.Store{Projects: []task.Project{{
		Name:  "gym",
		Items: []task.Item{{Text: "from gitlab", Priority: 2, GitLabIssue: 4, GitLabProject: "group/gym"}},
	}}}
	model := *m
	updated, _ := model.Update(gitlabSyncedMsg{
		reports: []syncReport{{Project: "gym", Path: "group/gym", Imported: 1}},
		store:   imported,
	})
	*m = updated.(uiModel)

	if len(m.items) != 1 || m.items[0].Text != "from gitlab" {
		t.Fatalf("model did not adopt the synced items: %+v", m.items)
	}
	if m.status != "gitlab: 1 new" {
		t.Errorf("status = %q, want a sync summary", m.status)
	}
	if len(m.visibleRows()) != 1 {
		t.Errorf("list not refreshed with the imported item")
	}
	if m.list.FilterState() != list.FilterApplied {
		t.Errorf("a background sync cleared the user's filter")
	}
}

func TestGitlabSyncedMsgReportsFailure(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".todo.yaml")
	m, _ := newGitlabModel(t, cfg)

	model := *m
	updated, _ := model.Update(gitlabSyncedMsg{
		reports: []syncReport{{Project: "gym", Path: "group/gym", Err: fmt.Errorf("gitlab: 401 Unauthorized")}},
	})
	*m = updated.(uiModel)
	if !strings.Contains(m.status, "gitlab: 1 of 1 links failed") || !strings.Contains(m.status, "401") {
		t.Errorf("status = %q", m.status)
	}
}

// link/unlink share the config-writing path with the page; the CLI
// must never persist a token that only exists in the environment.
func TestGitlabCLILinkUnlinkKeepsEnvOutOfTheFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TODO_GITLAB_TOKEN", "env-only-token")

	dataPath := filepath.Join(home, "tasks.json")
	if err := task.SaveStore(dataPath, task.Store{Projects: []task.Project{{Name: "gym"}}}); err != nil {
		t.Fatal(err)
	}
	viper.Set("datafile", dataPath)
	cfg := filepath.Join(home, ".todo.yaml")
	if err := os.WriteFile(cfg, []byte("# mine\ndatafile: /tmp/tasks.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := gitlabLinkRun(nil, []string{"gym", "group/gym"}); err != nil {
		t.Fatalf("gitlabLinkRun: %v", err)
	}
	raw, _ := os.ReadFile(cfg)
	if strings.Contains(string(raw), "env-only-token") {
		t.Errorf("environment token was written to the config:\n%s", raw)
	}
	if !strings.Contains(string(raw), "# mine") {
		t.Errorf("link clobbered the config file:\n%s", raw)
	}
	got := gitlab.LoadFileSettings(cfg)
	if got.Links["gym"] != "group/gym" {
		t.Fatalf("links = %v", got.Links)
	}

	if err := gitlabLinkRun(nil, []string{"missing", "group/missing"}); err == nil {
		t.Errorf("linking a missing td project succeeded")
	}
	if err := gitlabUnlinkRun(nil, []string{"gym"}); err != nil {
		t.Fatalf("gitlabUnlinkRun: %v", err)
	}
	if _, ok := gitlab.LoadFileSettings(cfg).Links["gym"]; ok {
		t.Errorf("unlink left the link in the config")
	}
	if err := gitlabUnlinkRun(nil, []string{"gym"}); err == nil {
		t.Errorf("unlinking twice succeeded")
	}
}

// syncLink must import issues and save them under the data-file lock,
// leaving other projects alone.
func TestSyncLinksImportsAndSaves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues"):
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"iid": 4, "title": "remote task", "description": "from gitlab",
				"state": "opened", "created_at": "2026-11-01T09:00:00Z",
			}})
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not found"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer srv.Close()

	dataPath := filepath.Join(t.TempDir(), "tasks.json")
	if err := task.SaveStore(dataPath, task.Store{Projects: []task.Project{
		{Name: "gym"},
		{Name: "inbox", Items: []task.Item{{Text: "keep", Priority: 2}}},
	}}); err != nil {
		t.Fatal(err)
	}

	settings := gitlab.Settings{
		URL: srv.URL, Token: "tok",
		Links: map[string]string{"gym": "group/gym"},
	}
	reports, store, err := syncLinks(dataPath, settings, nil)
	if err != nil {
		t.Fatalf("syncLinks: %v", err)
	}
	if len(reports) != 1 || reports[0].Err != nil || reports[0].Imported != 1 {
		t.Fatalf("reports = %+v", reports)
	}
	if got := reports[0].changeSummary(); got != "1 new" {
		t.Errorf("summary = %q", got)
	}

	item := store.Project("gym").Items[0]
	if item.Text != "remote task" || item.GitLabIssue != 4 || item.GitLabProject != "group/gym" {
		t.Errorf("imported item = %+v", item)
	}
	if want := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC); !item.Created.Equal(want) {
		t.Errorf("imported Created = %v, want %v", item.Created, want)
	}
	saved, err := task.ReadStore(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Project("gym").Items) != 1 || saved.Project("inbox").Items[0].Text != "keep" {
		t.Errorf("saved store = %+v", saved)
	}

	if _, _, err := syncLinks(dataPath, settings, []string{"inbox"}); err == nil {
		t.Errorf("syncing an unlinked project succeeded")
	}
}
