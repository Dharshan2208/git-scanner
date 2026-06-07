package aggregator

import (
	"context"
	"sort"
	"strconv"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

// Aggregate collects, deduplicates, and sorts findings from a results channel.
func Aggregate(ctx context.Context, results chan types.Finding) []types.Finding {
	var final []types.Finding
	seen := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			// Return partial results on cancellation.
			SortFindings(final)
			return final
		case res, ok := <-results:
			if !ok {
				SortFindings(final)
				return final
			}
			key := res.File + "|" + strconv.Itoa(res.Line) + "|" + res.Type + "|" + res.Match
			if !seen[key] {
				seen[key] = true
				final = append(final, res)
			}
		}
	}
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
