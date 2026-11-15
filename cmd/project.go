/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var projectForceOpt bool
var projectGroupOpt string

var projectCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"projects"},
	Short:   "Manage project namespaces.",
	Long: `Projects namespace todo items: every project keeps its own list and
its own numbering, so item 3 of one project never refers to another
project's items. Commands select a namespace with --project/-P; when
omitted they operate on the default project "inbox".`,
	RunE:          projectListRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectCreateCmd = &cobra.Command{
	Use:           "create NAME",
	Short:         "Create a new project.",
	Args:          cobra.ExactArgs(1),
	RunE:          projectCreateRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectDeleteCmd = &cobra.Command{
	Use:           "delete NAME",
	Short:         "Delete a project and all of its items.",
	Args:          cobra.ExactArgs(1),
	RunE:          projectDeleteRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectRenameCmd = &cobra.Command{
	Use:           "rename OLD NEW",
	Short:         "Rename a project.",
	Args:          cobra.ExactArgs(2),
	RunE:          projectRenameRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectGroupCmd = &cobra.Command{
	Use:           "group PROJECT GROUP",
	Short:         "Move a project into a group (created when missing).",
	Args:          cobra.ExactArgs(2),
	RunE:          projectGroupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectUngroupCmd = &cobra.Command{
	Use:           "ungroup PROJECT",
	Short:         "Remove a project from its group.",
	Args:          cobra.ExactArgs(1),
	RunE:          projectUngroupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectCreateGroupCmd = &cobra.Command{
	Use:           "create-group NAME",
	Short:         "Create an empty group.",
	Args:          cobra.ExactArgs(1),
	RunE:          projectCreateGroupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectRenameGroupCmd = &cobra.Command{
	Use:           "rename-group OLD NEW",
	Short:         "Rename a group.",
	Args:          cobra.ExactArgs(2),
	RunE:          projectRenameGroupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var projectDeleteGroupCmd = &cobra.Command{
	Use:           "delete-group NAME",
	Short:         "Delete a group; its projects are kept, ungrouped.",
	Args:          cobra.ExactArgs(1),
	RunE:          projectDeleteGroupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// resolveProject finds the project the current command operates on
// (see projectName). The default project always resolves: with ensure
// it is created in the store, without it a transient empty project is
// returned so read-only commands on a fresh file list nothing instead
// of erroring. Any other name must already exist.
func resolveProject(s *task.Store, ensure bool) (*task.Project, error) {
	name := projectName()
	if p := s.Project(name); p != nil {
		return p, nil
	}
	if name == task.DefaultProject {
		if ensure {
			return s.EnsureProject(name), nil
		}
		return &task.Project{Name: name}, nil
	}
	return nil, fmt.Errorf("no such project %q (create it: td project create %s)", name, name)
}

func projectListRun(cmd *cobra.Command, args []string) error {
	store, err := task.ReadStore(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 3, 0, 1, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tPENDING\tDONE\tTOTAL")
	active := projectName()
	row := func(indent, name string, p *task.Project) {
		done := 0
		for _, it := range p.Items {
			if it.Done {
				done++
			}
		}
		marker := " "
		if name == active {
			marker = "*"
		}
		fmt.Fprintf(w, "%s %s%s\t%d\t%d\t%d\n", marker, indent, name, len(p.Items)-done, done, len(p.Items))
	}
	if len(store.Groups) == 0 {
		for k := range store.Projects {
			row("", store.Projects[k].Name, &store.Projects[k])
		}
		return w.Flush()
	}
	// With groups the list becomes a tree: group rows carry the
	// aggregate counts, member projects are indented beneath them.
	for _, g := range store.Groups {
		pending, done, total := 0, 0, 0
		for _, n := range g.Projects {
			if p := store.Project(n); p != nil {
				for _, it := range p.Items {
					if it.Done {
						done++
					} else {
						pending++
					}
					total++
				}
			}
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", g.Name, pending, done, total)
		for _, n := range g.Projects {
			if p := store.Project(n); p != nil {
				row("  ", n, p)
			}
		}
	}
	ungrouped := false
	for k := range store.Projects {
		p := &store.Projects[k]
		if store.GroupOf(p.Name) != "" {
			continue
		}
		if !ungrouped {
			fmt.Fprintln(w, "(ungrouped)\t\t\t")
			ungrouped = true
		}
		row("  ", p.Name, p)
	}
	return w.Flush()
}

func projectCreateRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if err := task.ValidateProjectName(name); err != nil {
		return err
	}
	group := strings.TrimSpace(projectGroupOpt)
	if group != "" {
		if err := task.ValidateGroupName(group); err != nil {
			return err
		}
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
	if store.Project(name) != nil {
		return fmt.Errorf("project %q already exists", name)
	}
	store.EnsureProject(name)
	if group != "" {
		if err := store.SetGroup(name, group); err != nil {
			return err
		}
	}

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Created project %q\n", name)
	return nil
}

func projectDeleteRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])

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
	p := store.Project(name)
	if p == nil {
		return fmt.Errorf("no such project %q", name)
	}
	if len(p.Items) > 0 && !projectForceOpt {
		return fmt.Errorf("project %q has %d item(s); rerun with --force to delete them too", name, len(p.Items))
	}
	store.RemoveProject(name)

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	if len(p.Items) == 0 {
		fmt.Printf("Deleted project %q\n", name)
	} else {
		fmt.Printf("Deleted project %q (%d items)\n", name, len(p.Items))
	}
	return nil
}

func projectRenameRun(cmd *cobra.Command, args []string) error {
	oldName, newName := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])

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
	if err := store.RenameProject(oldName, newName); err != nil {
		return err
	}

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Renamed project %q to %q\n", oldName, newName)
	return nil
}

func projectGroupRun(cmd *cobra.Command, args []string) error {
	project := strings.TrimSpace(args[0])
	group := strings.TrimSpace(args[1])
	if err := task.ValidateGroupName(group); err != nil {
		return err
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
	if err := store.SetGroup(project, group); err != nil {
		return err
	}

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Moved project %q to group %q\n", project, group)
	return nil
}

func projectUngroupRun(cmd *cobra.Command, args []string) error {
	project := strings.TrimSpace(args[0])

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
	if err := store.SetGroup(project, ""); err != nil {
		return err
	}

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Ungrouped project %q\n", project)
	return nil
}

func projectCreateGroupRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if err := task.ValidateGroupName(name); err != nil {
		return err
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
	if store.Group(name) != nil {
		return fmt.Errorf("group %q already exists", name)
	}
	store.EnsureGroup(name)

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Created group %q\n", name)
	return nil
}

func projectRenameGroupRun(cmd *cobra.Command, args []string) error {
	oldName, newName := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
	if err := task.ValidateGroupName(newName); err != nil {
		return err
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
	g := store.Group(oldName)
	if g == nil {
		return fmt.Errorf("no such group %q", oldName)
	}
	if store.Group(newName) != nil {
		return fmt.Errorf("group %q already exists", newName)
	}
	g.Name = newName

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Renamed group %q to %q\n", oldName, newName)
	return nil
}

func projectDeleteGroupRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])

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
	g := store.Group(name)
	if g == nil {
		return fmt.Errorf("no such group %q", name)
	}
	members := len(g.Projects)
	store.RemoveGroup(name)

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	if members == 0 {
		fmt.Printf("Deleted group %q\n", name)
	} else {
		fmt.Printf("Deleted group %q (%d projects kept, now ungrouped)\n", name, members)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectCreateCmd, projectDeleteCmd, projectRenameCmd,
		projectGroupCmd, projectUngroupCmd, projectCreateGroupCmd, projectRenameGroupCmd, projectDeleteGroupCmd)
	projectDeleteCmd.Flags().BoolVar(&projectForceOpt, "force", false, "delete the project even when it still has items")
	projectCreateCmd.Flags().StringVarP(&projectGroupOpt, "group", "G", "", "create the project inside this group (created when missing)")
}
