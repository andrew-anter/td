/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"strconv"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var moveCmd = &cobra.Command{
	Use:     "move <n> <project>",
	Aliases: []string{"mv"},
	Short:   "Move item n from the current project to another project.",
	Long: `Move the nth item (as shown by 'list' in the source project, selected
with --project/-P) into the target project. The target must already
exist. The item keeps its priority and completion state; it is renumbered
by the target project's own display order.`,
	Args:          cobra.ExactArgs(2),
	RunE:          moveRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func moveRun(cmd *cobra.Command, args []string) error {
	i, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", args[0], err)
	}
	destName := args[1]

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
	src, err := resolveProject(&store, true)
	if err != nil {
		return err
	}

	if destName == src.Name {
		return fmt.Errorf("item %d is already in project %q", i, destName)
	}
	dest := store.Project(destName)
	if dest == nil {
		return fmt.Errorf("no such project %q (create it: td project create %s)", destName, destName)
	}

	if i < 1 || i > len(src.Items) {
		return fmt.Errorf("%d does not match any items in project %q", i, src.Name)
	}

	// Resolve the display number against the source project's sorted
	// order, exactly like complete/delete/edit/reopen do.
	target := sortedOrderIndices(src.Items)[i-1]
	moved := src.Items[target]
	src.Items = append(src.Items[:target], src.Items[target+1:]...)
	dest.Items = append(dest.Items, moved)

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("%q moved to %q\n", moved.Text, destName)
	return nil
}

func init() {
	rootCmd.AddCommand(moveCmd)
}
