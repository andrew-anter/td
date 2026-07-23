/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"todo/task"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	completedOpt bool
	allOpt       bool
	jsonOpt      bool
	countOpt     bool
)

// listOptions controls runList's filtering and output mode.
type listOptions struct {
	ShowAll       bool // include everything (pending + completed)
	ShowCompleted bool // include only completed items
	AsJSON        bool // emit items as a JSON array
	AsCount       bool // emit {"pending","completed","total"} as JSON
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List items in todo.",
	Long: `Listing the todo items in the datafile.

Output modes:
  default    Human-readable table (pending items, sorted by priority).
  --json     Emit the items as a JSON array, each item with a 1-based
             "Index" field matching the human list, in display order.
  --count    Emit a single JSON object {"pending":N,"completed":N,"total":N}.

Filters (default, --all, --completed) apply to both the human and --json
output. --count always reports totals across the whole data file.`,
	RunE:          listRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func listRun(cmd *cobra.Command, args []string) error {
	return runList(listOptions{
		ShowAll:       allOpt,
		ShowCompleted: completedOpt,
		AsJSON:        jsonOpt,
		AsCount:       countOpt,
	})
}

func runList(opts listOptions) error {
	items, err := task.ReadItems(viper.GetString("datafile"))
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	// --count is independent of sort/filter: report totals across all items.
	if opts.AsCount {
		var pending, done int
		for _, i := range items {
			if i.Done {
				done++
			} else {
				pending++
			}
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Pending   int `json:"pending"`
			Completed int `json:"completed"`
			Total     int `json:"total"`
		}{Pending: pending, Completed: done, Total: len(items)})
	}

	sort.Stable(task.ByPri(items))

	filtered := make([]task.Item, 0, len(items))
	for _, i := range items {
		if opts.ShowAll || i.Done == opts.ShowCompleted {
			filtered = append(filtered, i)
		}
	}

	if opts.AsJSON {
		type entry struct {
			Index    int    `json:"Index"`
			Text     string `json:"Text"`
			Priority int    `json:"Priority"`
			Done     bool   `json:"Done"`
		}
		out := make([]entry, len(filtered))
		for k, i := range filtered {
			out[k] = entry{Index: k + 1, Text: i.Text, Priority: i.Priority, Done: i.Done}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 3, 0, 1, ' ', 0)
	for idx, i := range filtered {
		fmt.Fprintf(w, "%d.\t%s\t%s\t%s\n", idx+1, i.PrettyDone(), i.PrettyP(), i.Text)
	}
	return w.Flush()
}

// sortedOrderIndices returns the original indices of items in the order
// produced by task.ByPri (undone first, then ascending priority, stable).
// The result has length len(items); result[0] is the index of the item
// that would appear first in `td list`.
func sortedOrderIndices(items []task.Item) []int {
	order := make([]int, len(items))
	for k := range order {
		order[k] = k
	}
	sort.SliceStable(order, func(a, b int) bool {
		return task.ByPri(items).Less(order[a], order[b])
	})
	return order
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&allOpt, "all", "a", false, "Show All Todos")
	listCmd.Flags().BoolVarP(&completedOpt, "completed", "d", false, "Show 'Completed' Todos")
	listCmd.Flags().BoolVar(&jsonOpt, "json", false, "Emit items as a JSON array (display order, with 1-based Index).")
	listCmd.Flags().BoolVar(&countOpt, "count", false, "Emit {\"pending\",\"completed\",\"total\"} as JSON.")
}
