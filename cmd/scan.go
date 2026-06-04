package cmd

import (
	"fmt"
	"log"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/lifecycle"
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
	history    bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a repository for secrets and APIs",
	Run: func(cmd *cobra.Command, args []string) {
		if repoURL != "" && localPath != "" {
			log.Fatal("Cannot specify both --repo and --local")
		}

		var input string
		switch {
		case repoURL != "":
			input = repoURL
		case localPath != "":
			input = localPath
		default:
			log.Fatal("Provide --local <path> or --repo <url>")
		}

		// --- Resolve path (clone if remote) ---
		path, cleanup, err := repo.Resolve(input)
		if err != nil {
			log.Fatal(err)
		}
		defer cleanup()

		fmt.Println("Resolved Path : ", path)

		// --- History mode ---
		if history {
			findings, err := lifecycle.RunParallelHistoryScan(path)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("\nHistory scan completed.")
			output.PrintFindings(findings, path)
			output.SaveReport(findings, path, outputFile, format)
			return
		}

		// --- Working tree mode ---
		jobs := make(chan worker.Job, 200)
		results := worker.StartWorkerPool(jobs)

		// Walk the filesystem in a goroutine, communicate errors via channel.
		// Using an error channel instead of log.Fatal inside the goroutine
		// ensures deferred cleanup() in the parent goroutine still runs.
		walkErr := make(chan error, 1)
		go func() {
			walkErr <- walker.Walk(path, jobs)
		}()

		// Aggregate blocks until the results channel is closed (all workers done).
		aggregatedFindings := aggregator.Aggregate(results)

		// Check whether the walker encountered an error.
		if err := <-walkErr; err != nil {
			log.Printf("Warning: walker encountered errors (partial results may be returned): %v", err)
		}

		output.PrintFindings(aggregatedFindings, path)
		output.SaveReport(aggregatedFindings, path, outputFile, format)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&localPath, "local", "", "Local directory to scan")
	scanCmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
	scanCmd.Flags().StringVar(&outputFile, "output", "", "Path to save report (e.g. report.md)")
	scanCmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
	scanCmd.Flags().BoolVar(&history, "history", false, "Scan git commit history")
}
