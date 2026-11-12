package gitlab

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The config file belongs to the user: saving the GitLab section must
// keep its comments, ordering, and unrelated keys.
func TestSaveSettingsPreservesUnrelatedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.yaml")
	writeFile(t, path, "# td settings\ndatafile: /home/you/.tasks.json\nproject: gym   # default namespace\n")

	want := Settings{
		URL:   "https://git.example.com",
		Token: "glpat-secret",
		Links: map[string]string{
			"gym":   "group/generator/gym",
			"inbox": "group/inbox",
		},
	}
	if err := SaveSettings(path, want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"# td settings",
		"datafile: /home/you/.tasks.json",
		"# default namespace",
		"gitlab_url: https://git.example.com",
		"gitlab_token: glpat-secret",
		"gitlab_projects:",
	} {
		if !strings.Contains(string(raw), s) {
			t.Errorf("saved config lost %q:\n%s", s, raw)
		}
	}
	if n := strings.Count(string(raw), "gitlab_projects"); n != 1 {
		t.Errorf("gitlab_projects written %d times:\n%s", n, raw)
	}

	got := LoadFileSettings(path)
	if got.URL != want.URL || got.Token != want.Token || !reflect.DeepEqual(got.Links, want.Links) {
		t.Errorf("reloaded settings = %+v, want %+v", got, want)
	}
}

// Saving twice updates the section in place instead of appending a
// second one, and removing every link drops the key.
func TestSaveSettingsReplacesSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.yaml")
	if err := SaveSettings(path, Settings{URL: "https://one.example.com", Token: "t1", Links: map[string]string{"a": "g/a"}}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := SaveSettings(path, Settings{URL: "https://two.example.com", Token: "t2", Links: map[string]string{"a": "g/b"}}); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got := LoadFileSettings(path)
	if got.URL != "https://two.example.com" || got.Token != "t2" || got.Links["a"] != "g/b" {
		t.Fatalf("settings after second save = %+v", got)
	}
	raw, _ := os.ReadFile(path)
	if n := strings.Count(string(raw), "gitlab_url"); n != 1 {
		t.Errorf("gitlab_url appears %d times:\n%s", n, raw)
	}

	if err := SaveSettings(path, Settings{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "gitlab_") {
		t.Errorf("cleared settings left keys behind:\n%s", raw)
	}
}

func TestSaveSettingsCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.yaml")
	if err := SaveSettings(path, Settings{URL: "https://git.example.com", Token: "tok"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	got := LoadFileSettings(path)
	if got.URL != "https://git.example.com" || got.Token != "tok" || len(got.Links) != 0 {
		t.Errorf("loaded = %+v", got)
	}
}

func TestSaveSettingsAcceptsEmptyAndNullFiles(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "null": "null\n"} {
		path := filepath.Join(t.TempDir(), ".todo.yaml")
		writeFile(t, path, content)
		if err := SaveSettings(path, Settings{URL: "https://git.example.com", Token: "tok"}); err != nil {
			t.Errorf("%s file: SaveSettings: %v", name, err)
		}
		if got := LoadFileSettings(path); got.URL != "https://git.example.com" {
			t.Errorf("%s file: loaded %+v", name, got)
		}
	}
}

// A config file that cannot be parsed is not silently rewritten: the
// user gets the parse error and keeps their file.
func TestSaveSettingsRejectsBrokenConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.yaml")
	writeFile(t, path, "- not\n- a mapping\n")
	err := SaveSettings(path, Settings{URL: "https://git.example.com"})
	if err == nil {
		t.Fatal("expected an error for a non-mapping config")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "- not\n- a mapping\n" {
		t.Errorf("broken config was rewritten:\n%s", raw)
	}
}

func TestLoadSettingsAppliesEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.yaml")
	writeFile(t, path, "gitlab_url: https://file.example.com\ngitlab_token: filetok\ngitlab_projects:\n  Gym: group/gym\n")

	file := LoadFileSettings(path)
	if file.URL != "https://file.example.com" || file.Token != "filetok" {
		t.Fatalf("file settings = %+v", file)
	}
	if got := file.Links["Gym"]; got != "group/gym" {
		t.Errorf("link keys must keep their case: %v", file.Links)
	}

	t.Setenv("TODO_GITLAB_URL", "https://env.example.com")
	t.Setenv("TODO_GITLAB_TOKEN", "")
	effective := LoadSettings(path)
	if effective.URL != "https://env.example.com" {
		t.Errorf("URL = %q, want the environment override", effective.URL)
	}
	if effective.Token != "filetok" {
		t.Errorf("Token = %q, want the file value", effective.Token)
	}
	if env := EnvOverrides(); !env.URL || env.Token {
		t.Errorf("EnvOverrides = %+v, want only URL", env)
	}
}

func TestSettingsLinkedTo(t *testing.T) {
	s := Settings{Links: map[string]string{"gym": "group/gym", "inbox": "group/inbox"}}
	if other, ok := s.LinkedTo("group/gym", "inbox"); !ok || other != "gym" {
		t.Errorf("LinkedTo = %q, %v", other, ok)
	}
	if other, ok := s.LinkedTo("group/gym", "gym"); ok {
		t.Errorf("LinkedTo ignored the except project: %q", other)
	}
	if _, ok := s.LinkedTo("group/other", ""); ok {
		t.Errorf("LinkedTo reported an unknown path")
	}
}

func TestValidateURLAndProjectPath(t *testing.T) {
	for _, ok := range []string{"https://git.example.com", "http://git.local:8080", "https://host/gitlab"} {
		if err := ValidateURL(ok); err != nil {
			t.Errorf("ValidateURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "git.example.com", "ftp://host", "https://"} {
		if err := ValidateURL(bad); err == nil {
			t.Errorf("ValidateURL(%q) accepted", bad)
		}
	}

	for _, ok := range []string{"name", "group/name", "/group/name/"} {
		if err := ValidateProjectPath(ok); err != nil {
			t.Errorf("ValidateProjectPath(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "group /name", "group/na me", "group\tname"} {
		if err := ValidateProjectPath(bad); err == nil {
			t.Errorf("ValidateProjectPath(%q) accepted", bad)
		}
	}
	if got := NormalizeProjectPath(" /group/name/ "); got != "group/name" {
		t.Errorf("NormalizeProjectPath = %q", got)
	}
}
