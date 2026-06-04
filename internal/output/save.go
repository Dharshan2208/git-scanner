package output

import (
	"fmt"
	"log"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

func SaveReport(findings []types.Finding, basePath, outputPath, format string) {
	if outputPath == "" {
		return
	}

	switch format {
	case "json", "JSON":
		if err := WriteJSON(findings, basePath, outputPath); err != nil {
			log.Printf("Failed to write JSON: %v", err)
		} else {
			fmt.Printf("JSON report saved to: %s\n", outputPath)
		}

	case "markdown", "md", "":
		if err := WriteMarkdown(findings, basePath, outputPath); err != nil {
			log.Printf("Failed to write markdown: %v", err)
		} else {
			fmt.Printf("Markdown report saved to: %s\n", outputPath)
		}

	default:
		log.Printf("Unknown format: %s. Supported: markdown, json", format)
	}
}
