package testharness

import (
	"slices"
	"strings"
)

// diffLines renders a plain line diff between two texts: unchanged lines are
// prefixed with two spaces, removals with "- ", additions with "+ ". Golden
// files are small and reviewed by humans, so a full LCS diff with complete
// context reads better than a windowed unified diff.
func diffLines(want, got string) string {
	a := splitLines(want)
	b := splitLines(got)

	// Longest-common-subsequence table.
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := range slices.Backward(a) {
		for j := range slices.Backward(b) {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var out strings.Builder
	writeLine := func(prefix, line string) {
		out.WriteString(prefix)
		out.WriteString(line)
		out.WriteByte('\n')
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			writeLine("  ", a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			writeLine("- ", a[i])
			i++
		default:
			writeLine("+ ", b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		writeLine("- ", a[i])
	}
	for ; j < len(b); j++ {
		writeLine("+ ", b[j])
	}
	return out.String()
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
