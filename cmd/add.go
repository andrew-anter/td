/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"todo/task"

	"github.com/spf13/cobra"
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new todo item to the list.",
	Long:  ``,
	Run:   addRun,
}

func addRun(cmd *cobra.Command, args []string) {
	items := []task.Item{}
	for _, x := range args {
		items = append(items, task.Item{Text: x})
	}

	err := task.SaveItems("./.tasks.json", items)
	if err != nil {
		_ = fmt.Errorf("%v", err)
	}
}

func init() {
	rootCmd.AddCommand(addCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// addCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// addCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
