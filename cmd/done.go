/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var doneCmd = &cobra.Command{
	Use:           "done",
	Aliases:       []string{"do"},
	Short:         "Mark Item as Done",
	RunE:          doneRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func doneRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("done requires an item number")
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
	order := make([]int, len(items))
	for k := range order {
		order[k] = k
	}
	sort.SliceStable(order, func(a, b int) bool {
		return task.ByPri(items).Less(order[a], order[b])
	})
	target := order[i-1]
	items[target].Done = true

	if err := task.SaveItems(viper.GetString("datafile"), items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("%q %s\n", items[target].Text, "marked done")
	return nil
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
