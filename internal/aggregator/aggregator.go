package aggregator

import (
	"sort"
	"strconv"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

// Aggregate collects, deduplicates, and sorts findings from a results channel.
func Aggregate(results chan types.Finding) []types.Finding {
	var final []types.Finding
	seen := make(map[string]bool)

	for res := range results {
		key := res.File + "|" + strconv.Itoa(res.Line) + "|" + res.Type + "|" + res.Match
		if !seen[key] {
			seen[key] = true
			final = append(final, res)
		}
	}

	SortFindings(final)
	return final
}

// SortFindings sorts findings by file path, then line number.
func SortFindings(findings []types.Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File == findings[j].File {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].File < findings[j].File
	})
}
