package cmd

import (
	"sort"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/list"
)

// searchTier orders how a query word met the text, best first. The
// bubbles default filter (sahilm/fuzzy) ranks by a subsequence score
// only, so a scattered letter match like "a-u-t-h" inside "update the
// tag" could outrank a literal keyword hit; keyword search needs
// literal matches on top and scattered ones only as a fallback.
type searchTier int

const (
	tierScattered searchTier = iota + 1
	tierSubstring
	tierWordStart
	tierPrefix
	tierExact
)

// searchScoreScale keeps one tier worth more than any position, gap,
// or length penalty can take away, so tiers never interleave.
const (
	searchScoreScale = 1 << 20
	searchPosCap     = 2000
	searchPosWeight  = 64
)

// relevanceFilter is the list.Filter used by the task list: every
// whitespace-separated word must match, results come back best first,
// and scattered subsequence matches are dropped entirely when some
// entry matches every word literally — otherwise searching a keyword
// buries the real hits under unrelated rows.
func relevanceFilter(term string, targets []string) []list.Rank {
	words := strings.Fields(strings.ToLower(term))
	if len(words) == 0 {
		return nil
	}

	type match struct {
		index int
		score int
		tier  searchTier
	}
	matches := make([]match, 0, len(targets))
	bestTier := searchTier(0)
	for i, t := range targets {
		score, tier, ok := scoreTarget([]rune(strings.ToLower(t)), words)
		if !ok {
			continue
		}
		matches = append(matches, match{index: i, score: score, tier: tier})
		if tier > bestTier {
			bestTier = tier
		}
	}

	if bestTier >= tierSubstring {
		literal := matches[:0]
		for _, m := range matches {
			if m.tier >= tierSubstring {
				literal = append(literal, m)
			}
		}
		matches = literal
	}

	sort.SliceStable(matches, func(a, b int) bool {
		return matches[a].score > matches[b].score
	})

	ranks := make([]list.Rank, len(matches))
	for k, m := range matches {
		ranks[k] = list.Rank{Index: m.index}
	}
	return ranks
}

// scoreTarget sums one score per query word and reports the weakest
// tier any of them reached.
func scoreTarget(text []rune, words []string) (int, searchTier, bool) {
	total := 0
	weakest := tierExact
	for _, w := range words {
		score, tier, ok := scoreWord(text, []rune(w))
		if !ok {
			return 0, 0, false
		}
		total += score
		if tier < weakest {
			weakest = tier
		}
	}
	return total, weakest, true
}

func scoreWord(text, word []rune) (int, searchTier, bool) {
	tier := tierScattered
	pos, span := 0, len(word)
	switch {
	case runsEqual(text, word):
		tier = tierExact
	case len(text) > len(word) && runsEqual(text[:len(word)], word):
		tier = tierPrefix
	default:
		if i := indexRun(text, word, true); i >= 0 {
			tier, pos = tierWordStart, i
		} else if i := indexRun(text, word, false); i >= 0 {
			tier, pos = tierSubstring, i
		} else {
			first, last, ok := subsequence(text, word)
			if !ok {
				return 0, 0, false
			}
			tier, pos, span = tierScattered, first, last-first+1
		}
	}

	score := int(tier) * searchScoreScale
	score -= min(pos, searchPosCap) * searchPosWeight
	if tier == tierScattered {
		// Fewer skipped runes means a tighter abbreviation.
		score -= min(span-len(word), searchPosCap) * searchPosWeight
	}
	score -= min(len(text), searchPosCap) // whole-word hits first among equals
	return score, tier, true
}

// indexRun finds word inside text, optionally only where it starts a
// word (text start or right after a non-alphanumeric rune).
func indexRun(text, word []rune, wordStart bool) int {
	for i := range len(text) - len(word) + 1 {
		if wordStart && i > 0 && isWordRune(text[i-1]) {
			continue
		}
		if runsEqual(text[i:i+len(word)], word) {
			return i
		}
	}
	return -1
}

// subsequence matches word's runes in order, but only from a position
// that starts a word: that anchor keeps a keyword from matching
// arbitrary letters scattered through an unrelated sentence. It returns
// the first and last matched rune offsets.
func subsequence(text, word []rune) (int, int, bool) {
	for i := range text {
		if text[i] != word[0] || (i > 0 && isWordRune(text[i-1])) {
			continue
		}
		k, j := 1, i+1
		for ; j < len(text) && k < len(word); j++ {
			if text[j] == word[k] {
				k++
			}
		}
		if k == len(word) {
			return i, j - 1, true
		}
	}
	return 0, 0, false
}

func runsEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
