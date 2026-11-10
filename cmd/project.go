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
	for _, p := range store.Projects {
		done := 0
		for _, it := range p.Items {
			if it.Done {
				done++
			}
		}
		marker := " "
		if p.Name == active {
			marker = "*"
		}
		fmt.Fprintf(w, "%s %s\t%d\t%d\t%d\n", marker, p.Name, len(p.Items)-done, done, len(p.Items))
	}
	return w.Flush()
}

func projectCreateRun(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if err := task.ValidateProjectName(name); err != nil {
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
	if store.Project(name) != nil {
		return fmt.Errorf("project %q already exists", name)
	}
	store.EnsureProject(name)

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
	if err := task.ValidateProjectName(newName); err != nil {
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
	p := store.Project(oldName)
	if p == nil {
		return fmt.Errorf("no such project %q", oldName)
	}
	if store.Project(newName) != nil {
		return fmt.Errorf("project %q already exists", newName)
	}
	p.Name = newName

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("Renamed project %q to %q\n", oldName, newName)
	return nil
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectCreateCmd, projectDeleteCmd, projectRenameCmd)
	projectDeleteCmd.Flags().BoolVar(&projectForceOpt, "force", false, "delete the project even when it still has items")
}
