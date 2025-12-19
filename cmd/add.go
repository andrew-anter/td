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

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new todo item to the list.",
	Long:  ``,
	Run:   addRun,
}

func addRun(cmd *cobra.Command, args []string) {
	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		fmt.Printf("%v", err)
		return
	}
	for _, x := range args {
		item := task.Item{Text: x}
		item.SetPriority(priority)
		items = append(items, item)
	}

	err = task.SaveItems(viper.GetString("datafile"), items)
	if err != nil {
		fmt.Printf("%v", err)
		return
	}
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVarP(&priority, "priority", "p", 2, "Priority:1,2,3")
}
