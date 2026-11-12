package cmd

import (
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/list"

	"todo/task"
)

func filterIndexes(term string, targets []string) []int {
	ranks := relevanceFilter(term, targets)
	out := make([]int, len(ranks))
	for i, r := range ranks {
		out[i] = r.Index
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Relevance ordering: an exact match beats a prefix match, which beats
// a word-start match, which beats a match buried mid-word — regardless
// of the order the rows arrive in.
func TestRelevanceFilterRanksLiteralMatchesByTier(t *testing.T) {
	targets := []string{
		"deployment notes",                    // prefix (idx 0)
		"set up auth",                         // exact word start (1)
		"reauthorize the token",               // mid-word substring (2)
		"auth",                                // exact (3)
		"Waterway, run the content extractor", // no match (4)
		"deploy the staging stack and auth gateway", // prefix + word start (5)
	}
	got := filterIndexes("auth", targets)
	want := []int{3, 1, 5, 2}
	if !equalInts(got, want) {
		t.Fatalf("ranked indexes = %v, want %v", got, want)
	}

	got = filterIndexes("deploy", targets)
	if !equalInts(got, []int{0, 5}) {
		t.Fatalf("deploy ranked indexes = %v, want [0 5]", got)
	}
}

// The bug report: a keyword search surfaced rows that only contained
// the letters scattered. Those must vanish while anything matches the
// word literally.
func TestRelevanceFilterDropsScatteredMatchesWhenKeywordHitsExist(t *testing.T) {
	targets := []string{
		"Upload space shall be automatically filled based on where i am",
		"Waterway email about updates and schedule a visit",
		"Setup Automatic Shutdown",
	}
	got := filterIndexes("email", targets)
	if !equalInts(got, []int{1}) {
		t.Fatalf("email ranked indexes = %v, want [1]", got)
	}
}

// With no literal hit, an abbreviation still finds its row.
func TestRelevanceFilterKeepsScatteredMatchesAsFallback(t *testing.T) {
	targets := []string{"something else entirely", "Waterway, fix netbird connection"}
	got := filterIndexes("wfnet", targets)
	if !equalInts(got, []int{1}) {
		t.Fatalf("abbreviation ranked indexes = %v, want [1]", got)
	}
}

// Multi-word queries require every word, and a word found in the
// description counts.
func TestRelevanceFilterRequiresEveryWord(t *testing.T) {
	targets := []string{
		"backup database before updating files\nnightly cron at 3am",
		"backup database before updating files",
		"update files",
	}
	got := filterIndexes("backup cron", targets)
	if !equalInts(got, []int{0}) {
		t.Fatalf("multi-word ranked indexes = %v, want [0]", got)
	}
}

// Case folding must not change the outcome.
func TestRelevanceFilterIsCaseInsensitive(t *testing.T) {
	targets := []string{"Review PR", "review pr"}
	got := filterIndexes("REVIEW", targets)
	if len(got) != 2 {
		t.Fatalf("case-insensitive search returned %d rows, want 2", len(got))
	}
}

// The list model must actually be handed the relevance filter; the
// bubbles default would rank the unrelated row first.
func TestUIModelSearchesWithRelevance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "Upload space shall be automatically filled based on where i am", Priority: 2},
		{Text: "Waterway email about updates", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")
	m.list.SetFilterText("email")

	vis := m.list.VisibleItems()
	if len(vis) != 1 {
		t.Fatalf("filter returned %d rows, want only the email task", len(vis))
	}
	if got := vis[0].(uiItem).row.item.Text; got != "Waterway email about updates" {
		t.Fatalf("top row = %q, want the literal email match", got)
	}
}

// Typing "/" then a keyword in the real model must leave the literal
// hit as the top row and hide the scattered-match noise.
func TestFilterKeystrokesRankRelevance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := task.Store{Projects: []task.Project{{Name: "inbox", Items: []task.Item{
		{Text: "Setup Automatic Shutdown", Priority: 2},
		{Text: "Waterway email about updates", Priority: 2},
		{Text: "Upload space shall be automatically filled based on where i am", Priority: 2},
	}}}}
	m := newUIModel(path, store, "inbox")

	for _, k := range []string{"/", "e", "m", "a", "i", "l"} {
		updated, _ := m.handleKey(keyMsg(k))
		m = updated.(uiModel)
	}
	if got := m.list.FilterInput.Value(); got != "email" {
		t.Fatalf("typed filter = %q, want %q", got, "email")
	}
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("FilterState = %v, want Filtering", m.list.FilterState())
	}

	m.list.SetFilterText(m.list.FilterInput.Value())
	vis := m.list.VisibleItems()
	if len(vis) != 1 {
		t.Fatalf("filter left %d rows, want only the literal email match", len(vis))
	}
	if got := vis[0].(uiItem).row.item.Text; got != "Waterway email about updates" {
		t.Fatalf("top row = %q, want the email task", got)
	}
}
