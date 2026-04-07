package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/output"
	"github.com/Dharshan2208/git-scanner/internal/repo"
	"github.com/Dharshan2208/git-scanner/internal/walker"
	"github.com/Dharshan2208/git-scanner/internal/worker"

	"github.com/spf13/cobra"
)

var (
	localPath  string
	repoURL    string
	outputFile string
	format     string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a repository for secrets and APIs",
	Run: func(cmd *cobra.Command, args []string) {
		var input string

		if repoURL != "" {
			input = repoURL
		} else {
			input = localPath
		}

		if input == "" {
			log.Fatal("Provide --local or --repo")
		}

		// Resolve path(clone if remote)
		path, cleanup, err := repo.Resolve(input)
		if err != nil {
			log.Fatal(err)
		}
		// auto cleanup the temp folder
		defer cleanup()

		fmt.Println("Resolved Path : ", path)

		// Creating the buffered job channel to prevent deadlock
		jobs := make(chan string, 200)

		// start worker pool that'll process files and return results
		results := worker.StartWorkerPool(jobs)

		// running the walker in goroutine and feeding the files into job channel
		go func() {
			if err := walker.Walk(path, jobs); err != nil {
				log.Fatal(err)
			}
		}()

		// Aggregate findinggs (deduplicate + sort)
		aggregatedFindings := aggregator.Aggregate(results)
		// Consume findings from workers
		foundCount := 0
		for _, finding := range aggregatedFindings {

			// converting the full path ..it's too long
			// to relative path
			relPath, _ := filepath.Rel(path, finding.File)
			fmt.Printf("[FOUND] %s | %s | Line No : %d\n",
				relPath,
				finding.Type,
				finding.Line,
			)
			foundCount++
		}

		fmt.Printf("Scanning completed.....\n")
		fmt.Printf("Total findings : %d\n", foundCount)

		if outputFile != "" {
			switch format {
			case "json", "JSON":
				if err := output.WriteJSON(aggregatedFindings, path, outputFile); err != nil {
					log.Printf("Warning: Failed to write JSON report: %v", err)
				} else {
					fmt.Printf("JSON report saved to: %s\n", outputFile)
				}

			case "markdown", "md", "":
				if err := output.WriteMarkdown(aggregatedFindings, path, outputFile); err != nil {
					log.Printf("Warning: Failed to write markdown report: %v", err)
				} else {
					fmt.Printf("Markdown report saved to: %s\n", outputFile)
				}

			default:
				log.Printf("Unknown format: %s. Supported: markdown, json", format)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&localPath, "local", "", "Local directory to scan")
	scanCmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	scanCmd.Flags().StringVar(&outputFile, "output", "", "Path to save markdown report(eg : report.md)")
	scanCmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
}
