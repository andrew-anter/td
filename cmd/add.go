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
	RunE:          addRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func addRun(cmd *cobra.Command, args []string) error {
	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}
	for _, x := range args {
		item := task.Item{Text: x}
		item.SetPriority(priority)
		items = append(items, item)
	}

	if err := task.SaveItems(viper.GetString("datafile"), items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVarP(&priority, "priority", "p", 2, "Priority:1,2,3")
}
