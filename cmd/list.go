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

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List items in todo.",
	Long:    `listing the todo items in the datafile.`,
	Run:     listRun,
}

func listRun(cmd *cobra.Command, args []string) {
	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		fmt.Printf("%v", err)
		return
	}

	sort.Sort(task.ByPri(items))
	w := tabwriter.NewWriter(os.Stdout, 3, 0, 1, ' ', 0)
	for _, i := range items {
		if allOpt || i.Done == doneOpt {
			fmt.Fprintln(w, i.Label()+"\t"+i.PrettyDone()+"\t"+i.PrettyP()+"\t"+i.Text+"\t")
		}
	}

	w.Flush()
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&allOpt, "all", "a", false, "Show All Todos")
	listCmd.Flags().BoolVarP(&doneOpt, "done", "d", false, "Show 'Done' Todos")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// listCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// listCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
