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

var deleteCompletedOpt bool

var deleteCmd = &cobra.Command{
	Use:           "delete",
	Aliases:       []string{"rm", "del"},
	Short:         "Delete tasks.",
	Long:          `Delete a specific task by its list number, or every completed task with --completed.`,
	RunE:          deleteRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func deleteRun(cmd *cobra.Command, args []string) error {
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
	items := project.Items

	if deleteCompletedOpt {
		if len(args) > 0 {
			return fmt.Errorf("--completed deletes all completed tasks; no item number expected")
		}
		items, err = deleteCompleted(items)
		if err != nil {
			return fmt.Errorf("failed to delete items: %w", err)
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("delete requires an item number.")
		}
		items, err = deleteByIndex(items, args[0])
		if err != nil {
			return fmt.Errorf("failed to delete item: %w", err)
		}
	}
	project.Items = items

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}

	return nil
}

// deleteCompleted removes every done item, keeping the rest in
// original file order. It reports how many items were deleted.
func deleteCompleted(items []task.Item) ([]task.Item, error) {
	kept := make([]task.Item, 0, len(items))
	for _, i := range items {
		if !i.Done {
			kept = append(kept, i)
		}
	}
	removed := len(items) - len(kept)
	switch removed {
	case 0:
		fmt.Println("No completed tasks to delete")
	case 1:
		fmt.Println("1 completed task deleted")
	default:
		fmt.Printf("%d completed tasks deleted\n", removed)
	}
	return kept, nil
}

func deleteByIndex(items []task.Item, arg string) ([]task.Item, error) {
	i, err := strconv.Atoi(arg)
	if err != nil {
		return items, fmt.Errorf("%q is not a valid label: %w", arg, err)
	}
	if i < 1 || i > len(items) {
		return items, fmt.Errorf("%d does not match any items", i)
	}

	target := sortedOrderIndices(items)[i-1]
	deleted := items[target]
	items = append(items[:target], items[target+1:]...)

	fmt.Printf("%q %s\n", deleted.Text, "deleted")
	return items, nil
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteCompletedOpt, "completed", false, "Delete every completed task (instead of a single item).")
}
