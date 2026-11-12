/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"testing"
)

func TestAddDescFlagStoresDescription(t *testing.T) {
	writeItems(t, nil)
	priority, descOpt = 2, "bring the receipt"
	t.Cleanup(func() { priority, descOpt = 0, "" })

	if err := addRun(nil, []string{"buy milk"}); err != nil {
		t.Fatalf("addRun: %v", err)
	}
	got := readSaved(t)
	if got[0].Text != "buy milk" || got[0].Description != "bring the receipt" {
		t.Fatalf("item = %+v, want the -D text stored", got[0])
	}
}

func TestAddWithoutDescStaysEmpty(t *testing.T) {
	writeItems(t, nil)
	priority, descOpt = 2, ""

	if err := addRun(nil, []string{"buy milk"}); err != nil {
		t.Fatalf("addRun: %v", err)
	}
	if got := readSaved(t); got[0].Description != "" {
		t.Fatalf("item = %+v, want no description", got[0])
	}
}
