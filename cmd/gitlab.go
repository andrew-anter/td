/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"todo/gitlab"
	"todo/task"
)

var gitlabCmd = &cobra.Command{
	Use:   "gitlab",
	Short: "Show the GitLab issue sync configuration.",
	Long: `Mirror a td project against a GitLab project: its open issues become
items, and completing or reopening an item closes or reopens its
issue. The integration is off until gitlab_url and gitlab_token are
set and at least one project is linked; td works unchanged without it.

Configure it in the TUI (press g) or from the CLI:
  td gitlab link gym group/generator/gym
  td gitlab sync`,
	RunE:          gitlabStatusRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var gitlabLinkCmd = &cobra.Command{
	Use:   "link TD_PROJECT GITLAB_PROJECT",
	Short: "Mirror a td project with a GitLab project.",
	Args:  cobra.ExactArgs(2),
	RunE:  gitlabLinkRun,
}

var gitlabUnlinkCmd = &cobra.Command{
	Use:   "unlink TD_PROJECT",
	Short: "Stop mirroring a td project with GitLab.",
	Args:  cobra.ExactArgs(1),
	RunE:  gitlabUnlinkRun,
}

var gitlabSyncCmd = &cobra.Command{
	Use:   "sync [TD_PROJECT...]",
	Short: "Sync linked projects with GitLab (all links by default).",
	Args:  cobra.ArbitraryArgs,
	RunE:  gitlabSyncRun,
}

var gitlabCreateCmd = &cobra.Command{
	Use:   "create N",
	Short: "Create a GitLab issue for item N and link it.",
	Long: `Create an issue in the linked GitLab project for item N of the
current td project and record the link, so later syncs keep the two
in step. Items are never pushed automatically.`,
	Args: cobra.ExactArgs(1),
	RunE: gitlabCreateRun,
}

func gitlabStatusRun(cmd *cobra.Command, args []string) error {
	cfgPath := configFilePath()
	file := gitlab.LoadFileSettings(cfgPath)
	env := gitlab.EnvOverrides()
	effective := file.WithEnv()

	if effective.URL != "" {
		fmt.Printf("url    %s%s\n", effective.URL, envNote(env.URL, "$TODO_GITLAB_URL"))
	}
	if effective.Token != "" {
		fmt.Printf("token  %s%s\n", maskToken(effective.Token), envNote(env.Token, "$TODO_GITLAB_TOKEN"))
	}
	if !effective.Configured() {
		fmt.Println("GitLab sync: not configured")
		fmt.Printf("  set gitlab_url and gitlab_token in %s, or press g in the TUI\n", cfgPath)
	}

	if len(file.Links) == 0 {
		fmt.Println("links  none — td gitlab link <td-project> <group/project>")
		return nil
	}
	fmt.Println("links")
	for _, name := range sortedLinks(file.Links) {
		fmt.Printf("  %-20s %s\n", name, file.Links[name])
	}
	return nil
}

func gitlabLinkRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	path := gitlab.NormalizeProjectPath(args[1])
	if err := task.ValidateProjectName(name); err != nil {
		return err
	}
	if err := gitlab.ValidateProjectPath(path); err != nil {
		return err
	}

	store, err := task.ReadStore(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}
	if store.Project(name) == nil {
		return fmt.Errorf("no such project %q (create it: td project create %s)", name, name)
	}

	// The file's own settings, so an exported TODO_GITLAB_TOKEN is not
	// copied into the config.
	settings := gitlab.LoadFileSettings(configFilePath())
	if settings.Links == nil {
		settings.Links = map[string]string{}
	}
	if other, ok := settings.LinkedTo(path, name); ok {
		return fmt.Errorf("GitLab project %s is already linked to td project %q", path, other)
	}
	settings.Links[name] = path
	if err := gitlab.SaveSettings(configFilePath(), settings); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("Linked project %q to GitLab project %s\n", name, path)
	return nil
}

func gitlabUnlinkRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	settings := gitlab.LoadFileSettings(configFilePath())
	if _, ok := settings.Link(name); !ok {
		return fmt.Errorf("project %q is not linked to GitLab", name)
	}
	delete(settings.Links, name)
	if err := gitlab.SaveSettings(configFilePath(), settings); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("Unlinked project %q\n", name)
	return nil
}

func gitlabSyncRun(cmd *cobra.Command, args []string) error {
	settings := gitlab.LoadSettings(configFilePath())
	if !settings.Configured() {
		return fmt.Errorf("GitLab is not configured (set gitlab_url and gitlab_token in %s, or press g in the TUI)", configFilePath())
	}

	reports, _, err := syncLinks(viper.GetString("datafile"), settings, args)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		return fmt.Errorf("no GitLab links configured (td gitlab link <td-project> <group/project>)")
	}

	failed := 0
	for _, r := range reports {
		fmt.Println(r.line())
		if r.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d links failed", failed, len(reports))
	}
	return nil
}

func gitlabCreateRun(cmd *cobra.Command, args []string) error {
	i, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", args[0], err)
	}

	path := viper.GetString("datafile")
	unlock, err := task.Lock(path)
	if err != nil {
		return fmt.Errorf("failed to lock data file: %w", err)
	}
	defer unlock.Close()

	store, err := task.ReadStore(path)
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}
	project, err := resolveProject(&store, true)
	if err != nil {
		return err
	}
	if i < 1 || i > len(project.Items) {
		return fmt.Errorf("%d does not match any items in project %q", i, project.Name)
	}
	target := sortedOrderIndices(project.Items)[i-1]
	item := &project.Items[target]
	if item.Linked() {
		return fmt.Errorf("%q already mirrors %s#%d", item.Text, item.GitLabProject, item.GitLabIssue)
	}

	settings := gitlab.LoadSettings(configFilePath())
	glPath, ok := settings.Link(project.Name)
	if !ok {
		return fmt.Errorf("project %q is not linked to GitLab (td gitlab link %s group/project)", project.Name, project.Name)
	}
	client, err := gitlab.NewClient(settings.URL, settings.Token)
	if err != nil {
		return err
	}
	ctx := context.Background()
	issue, err := client.CreateIssue(ctx, glPath, item.Text, item.Description)
	if err != nil {
		return err
	}

	item.GitLabIssue, item.GitLabProject = issue.IID, glPath
	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("issue created but the link was not saved: %w", err)
	}
	fmt.Printf("Created %s#%d for %q\n", glPath, issue.IID, item.Text)

	if item.Done {
		if err := client.SetIssueState(ctx, glPath, issue.IID, "close"); err != nil {
			return fmt.Errorf("issue created and linked, but closing it failed: %w", err)
		}
	}
	return nil
}

// syncReport is the outcome of syncing one link.
type syncReport struct {
	Project   string
	Path      string
	Imported  int
	Completed int
	Closed    int
	Reopened  int
	Skipped   int
	Err       error
}

// line renders the report for the CLI.
func (r syncReport) line() string {
	if r.Err != nil {
		return fmt.Sprintf("%s (%s): %v", r.Project, r.Path, r.Err)
	}
	return fmt.Sprintf("%s (%s): %s", r.Project, r.Path, r.changeSummary())
}

// changeSummary lists what changed, or "up to date".
func (r syncReport) changeSummary() string {
	var parts []string
	for _, c := range []struct {
		n    int
		word string
	}{
		{r.Imported, "new"},
		{r.Completed, "completed"},
		{r.Closed, "closed"},
		{r.Reopened, "reopened"},
		{r.Skipped, "issue missing"},
	} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.word))
		}
	}
	if len(parts) == 0 {
		return "up to date"
	}
	return strings.Join(parts, ", ")
}

// syncLinks syncs every link, or only the named td projects, and
// returns the store as it ended on disk. Links are independent: one
// failing does not stop the others.
func syncLinks(dataPath string, settings gitlab.Settings, only []string) ([]syncReport, task.Store, error) {
	if len(only) > 0 {
		var missing []string
		for _, name := range only {
			if _, ok := settings.Link(name); !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return nil, task.Store{}, fmt.Errorf("not linked to GitLab: %s", strings.Join(missing, ", "))
		}
	}

	want := make(map[string]bool, len(only))
	for _, name := range only {
		want[strings.TrimSpace(name)] = true
	}

	reports := make([]syncReport, 0, len(settings.Links))
	for _, name := range sortedLinks(settings.Links) {
		if len(want) > 0 && !want[name] {
			continue
		}
		reports = append(reports, syncLink(dataPath, settings, name, settings.Links[name]))
	}

	store, err := task.ReadStore(dataPath)
	if err != nil {
		return reports, task.Store{}, fmt.Errorf("failed to read items: %w", err)
	}
	return reports, store, nil
}

// syncLink syncs one link in two phases. The remote phase runs without
// the data-file lock so a slow instance never blocks the TUI from
// saving; the local phase then merges into a freshly read store under
// the lock.
func syncLink(dataPath string, settings gitlab.Settings, tdProject, glPath string) syncReport {
	rep := syncReport{Project: tdProject, Path: glPath}

	client, err := gitlab.NewClient(settings.URL, settings.Token)
	if err != nil {
		rep.Err = err
		return rep
	}

	snapshot, err := task.ReadStore(dataPath)
	if err != nil {
		rep.Err = fmt.Errorf("failed to read items: %w", err)
		return rep
	}
	if snapshot.Project(tdProject) == nil {
		rep.Err = fmt.Errorf("no such project %q", tdProject)
		return rep
	}

	res, err := gitlab.SyncRemote(context.Background(), client, glPath, gitlab.LocalItems(&snapshot, glPath))
	rep.Closed, rep.Reopened, rep.Skipped = res.Closed, res.Reopened, res.Skipped
	if err != nil {
		rep.Err = err
		return rep
	}

	unlock, err := task.Lock(dataPath)
	if err != nil {
		rep.Err = fmt.Errorf("failed to lock data file: %w", err)
		return rep
	}
	defer unlock.Close()

	store, err := task.ReadStore(dataPath)
	if err != nil {
		rep.Err = fmt.Errorf("failed to read items: %w", err)
		return rep
	}
	applied := gitlab.Apply(&store, tdProject, glPath, res)
	rep.Imported, rep.Completed = applied.Imported, applied.Completed
	if applied.Changed() {
		if err := task.SaveStore(dataPath, store); err != nil {
			rep.Err = fmt.Errorf("failed to save items: %w", err)
		}
	}
	return rep
}

// sortedLinks lists the linked td projects in a stable order.
func sortedLinks(links map[string]string) []string {
	names := make([]string, 0, len(links))
	for name := range links {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// maskToken keeps a token readable enough to tell two apart without
// printing it whole.
func maskToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "(not set)"
	}
	r := []rune(tok)
	if len(r) <= 4 {
		return strings.Repeat("•", len(r))
	}
	return "set (••••" + string(r[len(r)-4:]) + ")"
}

func envNote(overridden bool, name string) string {
	if !overridden {
		return ""
	}
	return "  (overridden by " + name + ")"
}

func init() {
	rootCmd.AddCommand(gitlabCmd)
	gitlabCmd.AddCommand(gitlabLinkCmd, gitlabUnlinkCmd, gitlabSyncCmd, gitlabCreateCmd)
}
