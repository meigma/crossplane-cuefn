// Package textdiff renders plain line diffs of small, human-reviewed texts
// such as golden files. It is shared by the test harness (want.yaml
// mismatches) and the static check core (XRD golden drift).
package textdiff

import (
	"slices"
	"strings"
)

// Lines renders a plain line diff between two texts: unchanged lines are
// prefixed with two spaces, removals with "- ", additions with "+ ". Golden
// files are small and reviewed by humans, so a full LCS diff with complete
// context reads better than a windowed unified diff.
func Lines(want, got string) string {
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

// Normalize strips carriage returns and enforces one trailing newline so
// golden comparison is stable across platforms and editors.
func Normalize(data []byte) string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
