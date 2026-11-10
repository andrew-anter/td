/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var priority int

var addCmd = &cobra.Command{
	Use:           "add",
	Short:         "Add a new todo item to the list.",
	Args:          cobra.MinimumNArgs(1),
	RunE:          addRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func addRun(cmd *cobra.Command, args []string) error {
	path := viper.GetString("datafile")

	// Validate the priority before touching the data file, so a bad
	// flag cannot leave a half-applied append behind.
	var probe task.Item
	if err := probe.SetPriority(priority); err != nil {
		return err
	}

	unlock, err := task.Lock(path)
	if err != nil {
		return fmt.Errorf("failed to lock data file: %w", err)
	}
	defer unlock.Close()

	items, err := task.ReadItems(path)
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}
	for _, x := range args {
		item := task.Item{Text: x}
		if err := item.SetPriority(priority); err != nil {
			return err
		}
		items = append(items, item)
	}

	if err := task.SaveItems(path, items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVarP(&priority, "priority", "p", 2, "Priority:1,2,3")
}
