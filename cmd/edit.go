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

var editCmd = &cobra.Command{
	Use:           "edit",
	Short:         "Edit the title of a task.",
	RunE:          editRun,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.ExactArgs(2),
}

func editRun(cmd *cobra.Command, args []string) error {
	path := viper.GetString("datafile")

	unlock, err := task.Lock(path)
	if err != nil {
		return fmt.Errorf("failed to lock data file: %w", err)
	}
	defer unlock.Close()

	items, err := task.ReadItems(path)
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	if err := editTaskTitle(items, args[0], args[1]); err != nil {
		return fmt.Errorf("failed to edit item: %w", err)
	}

	if err := task.SaveItems(path, items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	return nil
}

func editTaskTitle(items []task.Item, index string, newTitle string) error {
	i, err := strconv.Atoi(index)
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", index, err)
	}

	if i < 1 || i > len(items) {
		return fmt.Errorf("%d does not match any items", i)
	}

	target := sortedOrderIndices(items)[i-1]
	items[target].Text = newTitle

	return nil
}

func init() {
	rootCmd.AddCommand(editCmd)
}
