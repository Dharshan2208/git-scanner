package cmd

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/git"
	"github.com/Dharshan2208/git-scanner/internal/output"
	"github.com/Dharshan2208/git-scanner/internal/repo"
	"github.com/Dharshan2208/git-scanner/internal/walker"
	"github.com/Dharshan2208/git-scanner/internal/worker"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/spf13/cobra"
)

var (
	localPath  string
	repoURL    string
	outputFile string
	format     string
	history    bool
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

		if history {
			runHistoryScan(path)
			return
		}

		// Creating the buffered job channel to prevent deadlock
		jobs := make(chan worker.Job, 200)

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
		printFindings(aggregatedFindings, path)
		saveReport(aggregatedFindings, path)
	},
}

func runHistoryScan(repoPath string) {
	fmt.Println("Starting full git history scan... This may take a while.")

	var allFindings []worker.Finding

	err := git.ScanHistory(repoPath, func(commitInfo git.CommitInfo, tree *object.Tree) {
		// Create fresh channels for this commit
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)

		// Walk the *tree* instead of filesystem
		go func() {
			defer close(jobs)
			if err := walker.WalkTree(tree, repoPath, jobs); err != nil { // <-- new function needed
				log.Printf("Warning: walker failed for commit %s: %v", commitInfo.Hash[:8], err)
			}
		}()

		commitFindings := aggregator.Aggregate(results)

		// Attach commit info
		for i := range commitFindings {
			commitFindings[i].Commit = commitInfo.Hash
			commitFindings[i].Message = commitInfo.Message
		}

		allFindings = append(allFindings, commitFindings...)
	})
	if err != nil {
		log.Fatal("History scan failed:", err)
	}

	fmt.Println("\nHistory scan completed.")
	printFindings(allFindings, repoPath)
	saveReport(allFindings, repoPath)
}

// Helper functions to reduce congetion in the above code
func printFindings(findings []worker.Finding, basePath string) {
	foundCount := 0
	for _, f := range findings {
		relPath, _ := filepath.Rel(basePath, f.File)
		fmt.Printf("[FOUND] %s | %s | Line : %d\n", relPath, f.Type, f.Line)
		if f.Commit != "" {
			fmt.Printf("       Commit: %s | %s\n", f.Commit[:8], f.Message)
		}
		foundCount++
	}

	fmt.Printf("Scanning completed.....\n")
	fmt.Printf("Total findings : %d\n", foundCount)
}

func saveReport(findings []worker.Finding, basePath string) {
	if outputFile == "" {
		return
	}

	switch format {
	case "json", "JSON":
		if err := output.WriteJSON(findings, basePath, outputFile); err != nil {
			log.Printf("Failed to write JSON: %v", err)
		} else {
			fmt.Printf("JSON report saved to: %s\n", outputFile)
		}
	case "markdown", "md", "":
		if err := output.WriteMarkdown(findings, basePath, outputFile); err != nil {
			log.Printf("Failed to write markdown: %v", err)
		} else {
			fmt.Printf("Markdown report saved to: %s\n", outputFile)
		}
	default:
		log.Printf("Unknown format: %s. Supported: markdown, json", format)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&localPath, "local", "", "Local directory to scan")
	scanCmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	scanCmd.Flags().StringVar(&outputFile, "output", "", "Path to save markdown report(eg : report.md)")
	scanCmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
	scanCmd.Flags().BoolVar(&history, "history", false, "Scan git commit history")
}
