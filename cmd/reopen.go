/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"strconv"
	"time"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var reopenCmd = &cobra.Command{
	Use:           "reopen",
	Aliases:       []string{"uncomplete"},
	Short:         "Mark an item as not complete (reopen).",
	Long:          `Mark the nth item (as shown by 'list') as not complete. No-op if it is already pending.`,
	RunE:          reopenRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func reopenRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("reopen requires an item number")
	}

	i, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%q is not a valid label: %w", args[0], err)
	}

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

	if i < 1 || i > len(items) {
		return fmt.Errorf("%d does not match any items in project %q", i, project.Name)
	}

	// Find the original index of the i-th item as it would appear in
	// a sorted list, without mutating the loaded slice.
	target := sortedOrderIndices(items)[i-1]
	if !items[target].Done {
		// Already pending; treat as a quiet success so scripts can
		// call `td reopen N` unconditionally.
		return nil
	}
	items[target].MarkDone(false, time.Now())

	if err := task.SaveStore(path, store); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}
	fmt.Printf("%q %s\n", items[target].Text, "reopened")
	return nil
}

func init() {
	rootCmd.AddCommand(reopenCmd)
}
