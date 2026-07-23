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

var completeCmd = &cobra.Command{
	Use:           "complete",
	Aliases:       []string{"c", "comp"},
	Short:         "Mark Item as Complete",
	RunE:          completeRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func completeRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("complete requires an item number")
	}

	i, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", args[0], err)
	}

	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	if i < 1 || i > len(items) {
		return fmt.Errorf("%d does not match any items", i)
	}

	// Find the original index of the i-th item as it would appear in
	// a sorted list, without mutating the loaded slice. Saving in
	// original order keeps indices stable across runs.
	target := sortedOrderIndices(items)[i-1]
	items[target].Done = true

	if err := task.SaveItems(viper.GetString("datafile"), items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("%q %s\n", items[target].Text, "marked complete")
	return nil
}

func init() {
	rootCmd.AddCommand(completeCmd)
}
