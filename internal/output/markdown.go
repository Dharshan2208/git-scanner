package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

type Finding = types.Finding

// WriteMarkdown writes findings into a markdown report
func WriteMarkdown(findings []types.Finding, basePath, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "# Git Scanner Report\n")

	if len(findings) == 0 {
		fmt.Fprintln(file, "No secrets found...Shit")
		return nil
	}

	// Group findings by Type
	grouped := make(map[string][]types.Finding)
	for _, f := range findings {
		grouped[f.Type] = append(grouped[f.Type], f)
	}

	// Sort types
	var typesList []string
	for t := range grouped {
		typesList = append(typesList, t)
	}
	sort.Strings(typesList)

	// Write sections
	for _, t := range typesList {
		fmt.Fprintf(file, "## %s\n\n", t)

		for _, f := range grouped[t] {
			relPath, err := filepath.Rel(basePath, f.File)
			if err != nil {
				relPath = f.File // fallback to full path
			}
			fmt.Fprintf(file, "- **File:** `%s`  \n", relPath)
			fmt.Fprintf(file, "  **Line:** %d  \n", f.Line)
			fmt.Fprintf(file, "  **Match:** `%s`\n\n", f.Match)
		}

		fmt.Fprintf(file, "---\n")
	}

	fmt.Fprintf(file, "**Total unique findings:** %d\n", len(findings))

	return nil
}
