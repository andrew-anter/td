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

var deleteCmd = &cobra.Command{
	Use:           "delete",
	Aliases:       []string{"rm", "del"},
	Short:         "Delete tasks.",
	Long:          `Delete a specific task by its list number.`,
	RunE:          deleteRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func deleteRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("delete requires an item number.")
	}

	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	items, err = deleteByIndex(items, args[0])
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	if err := task.SaveItems(viper.GetString("datafile"), items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}

	return nil
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
}
