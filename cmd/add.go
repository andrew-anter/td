/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"todo/task"
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new todo item to the list.",
	Long:  ``,
	Run:   addRun,
}

func addRun(cmd *cobra.Command, args []string) {
	items, err := task.ReadItems(dataFile)
	if err != nil {
		fmt.Printf("%v", err)
		return
	}
	for _, x := range args {
		items = append(items, task.Item{Text: x})
	}

	err = task.SaveItems(dataFile, items)
	if err != nil {
		fmt.Printf("%v", err)
		return
	}
	fmt.Println("Items saved")
	fmt.Println(items)
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
