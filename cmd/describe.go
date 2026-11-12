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

var describeCmd = &cobra.Command{
	Use:           "describe",
	Aliases:       []string{"desc"},
	Short:         "Set the description of a task.",
	Long:          "Set the description of a task. Pass an empty string to clear it.",
	RunE:          describeRun,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.ExactArgs(2),
}

func describeRun(cmd *cobra.Command, args []string) error {
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

	if err := setTaskDescription(project.Items, args[0], args[1]); err != nil {
		return fmt.Errorf("failed to describe item: %w", err)
	}

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	return nil
}

func setTaskDescription(items []task.Item, index string, description string) error {
	i, err := strconv.Atoi(index)
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", index, err)
	}

	if i < 1 || i > len(items) {
		return fmt.Errorf("%d does not match any items", i)
	}

	target := sortedOrderIndices(items)[i-1]
	items[target].Description = description

	return nil
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
