/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	doneOpt bool
	allOpt  bool
)

var listCmd = &cobra.Command{
	Use:           "list",
	Aliases:       []string{"ls"},
	Short:         "List items in todo.",
	Long:          `listing the todo items in the datafile.`,
	RunE:          listRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func listRun(cmd *cobra.Command, args []string) error {
	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	sort.Stable(task.ByPri(items))
	w := tabwriter.NewWriter(os.Stdout, 3, 0, 1, ' ', 0)
	for idx, i := range items {
		if allOpt || i.Done == doneOpt {
			fmt.Fprintf(w, "%d.\t%s\t%s\t%s\n", idx+1, i.PrettyDone(), i.PrettyP(), i.Text)
		}
	}

	w.Flush()
	return nil
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&allOpt, "all", "a", false, "Show All Todos")
	listCmd.Flags().BoolVarP(&doneOpt, "done", "d", false, "Show 'Done' Todos")
}
