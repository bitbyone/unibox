package cli

import (
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// fuzzyScore says how well text matches what was typed: every letter of
// query in text in its order, case aside, letters side by side and at a
// word's start counting more; -1 when it does not match, 0 for nothing
// typed.
func fuzzyScore(query, text string) int {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(q) == 0 {
		return 0
	}
	t := []rune(strings.ToLower(ansi.Strip(text)))
	score, j, last := 0, 0, -2
	for i := 0; i < len(t) && j < len(q); i++ {
		if q[j] == ' ' {
			j++
			if j == len(q) {
				break
			}
		}
		if t[i] != q[j] {
			continue
		}
		score++
		if i == last+1 {
			score += 4
		}
		if i == 0 || !unicode.IsLetter(t[i-1]) && !unicode.IsDigit(t[i-1]) {
			score += 2
		}
		last = i
		j++
	}
	if j < len(q) {
		return -1
	}
	return score
}

// fuzzyFilter is the indexes of texts that match query, the best first and
// otherwise in their order; with nothing typed, all of them.
func fuzzyFilter(query string, texts []string) []int {
	type hit struct{ i, score int }
	hits := []hit{}
	for i, t := range texts {
		if s := fuzzyScore(query, t); s >= 0 {
			hits = append(hits, hit{i, s})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	out := make([]int, len(hits))
	for k, h := range hits {
		out[k] = h.i
	}
	return out
}
