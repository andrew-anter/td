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

var (
	deleteCompletedOpt bool
)

var deleteCmd = &cobra.Command{
	Use:           "delete",
	Aliases:       []string{"rm", "del"},
	Short:         "Delete tasks.",
	Long:          `Delete a specific task by its list number, or pass --completed to remove all completed tasks.`,
	RunE:          deleteRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func deleteRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && !deleteCompletedOpt {
		return fmt.Errorf("delete requires an item number or --completed")
	}

	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	if len(args) > 0 {
		return deleteByIndex(items, args[0])
	}
	return deleteCompleted(items)
}

func deleteByIndex(items []task.Item, arg string) error {
	i, err := strconv.Atoi(arg)
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", arg, err)
	}
	if i < 1 || i > len(items) {
		return fmt.Errorf("%d does not match any items", i)
	}

	target := sortedOrderIndices(items)[i-1]
	deleted := items[target]
	items = append(items[:target], items[target+1:]...)

	if err := task.SaveItems(viper.GetString("datafile"), items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("%q %s\n", deleted.Text, "deleted")
	return nil
}

func deleteCompleted(items []task.Item) error {
	kept := make([]task.Item, 0, len(items))
	deleted := 0
	for _, i := range items {
		if i.Done {
			deleted++
		} else {
			kept = append(kept, i)
		}
	}

	if err := task.SaveItems(viper.GetString("datafile"), kept); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	if deleted == 1 {
		fmt.Printf("deleted %d completed item\n", deleted)
	} else {
		fmt.Printf("deleted %d completed items\n", deleted)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteCompletedOpt, "completed", false, "Delete all completed tasks.")
}
