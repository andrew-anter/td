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

	store, err := task.ReadStore(path)
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}
	project, err := resolveProject(&store, true)
	if err != nil {
		return err
	}

	if err := editTaskTitle(project.Items, args[0], args[1]); err != nil {
		return fmt.Errorf("failed to edit item: %w", err)
	}

	if err := task.SaveStore(path, store); err != nil {
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
