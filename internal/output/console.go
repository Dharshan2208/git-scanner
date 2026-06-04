package output

import (
	"fmt"
	"path/filepath"

	"github.com/Dharshan2208/git-scanner/internal/types"
	"github.com/Dharshan2208/git-scanner/internal/utils"
)

func PrintFindings(findings []types.Finding, basePath string) {
	foundCount := 0
	for _, f := range findings {
		relPath, err := filepath.Rel(basePath, f.File)
		if err != nil {
			relPath = f.File
		}
		sanitizedMatch := utils.SanitizeSecret(f.Match)
		fmt.Printf("[FOUND] %s | %s | Line : %d\n", relPath, f.Type, f.Line)
		fmt.Printf("       Match: %s\n", sanitizedMatch)
		if f.Commit != "" {
			short := utils.ShortHash(f.Commit, 8)
			fmt.Printf("       Commit: %s | %s\n", short, f.Message)
		}
		foundCount++
	}

	fmt.Printf("Scanning completed.....\n")
	fmt.Printf("Total findings : %d\n", foundCount)
}
