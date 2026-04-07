package aggregator

import (
	"sort"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

// Aggregate collects, deduplicates, and sorts findings
func Aggregate(results chan types.Finding) []types.Finding {

	var final []types.Finding

	seen := make(map[string]bool)

	for res := range results {

		// unique key
		key := res.File + "|" + res.Match

		if !seen[key] {
			seen[key] = true
			final = append(final, res)
		}
	}

	// Optional: sort results (by file name)
	sort.Slice(final, func(i, j int) bool {
		if final[i].File == final[j].File {
			return final[i].Line < final[j].Line
		}
		return final[i].File < final[j].File
	})

	return final
}