/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var completedCmd = &cobra.Command{
	Use:           "completed",
	Short:         "Show completed todos.",
	RunE:          completedRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func completedRun(cmd *cobra.Command, args []string) error {
	return runList(false, true)
}

func init() {
	rootCmd.AddCommand(completedCmd)
}
