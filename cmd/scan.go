package cmd

import (
	"context"
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

// runScan contains the core scan logic and accepts a context for cancellation.
func runScan(ctx context.Context) error {
	if repoURL != "" && localPath != "" {
		return fmt.Errorf("cannot specify both --repo and --local")
	}

	var input string
	switch {
	case repoURL != "":
		input = repoURL
	case localPath != "":
		input = localPath
	default:
		return fmt.Errorf("provide --local <path> or --repo <url>")
	}

	path, cleanup, err := repo.Resolve(ctx, input)
	if err != nil {
		return fmt.Errorf("resolve failed: %w", err)
	}
	defer cleanup()

	fmt.Println("Resolved Path : ", path)

	if history {
		findings, err := lifecycle.RunParallelHistoryScan(ctx, path)
		if err != nil {
			return fmt.Errorf("history scan failed: %w", err)
		}
		fmt.Println("\nHistory scan completed.")
		output.PrintFindings(findings, path)
		output.SaveReport(findings, path, outputFile, format)
		return nil
	}

	jobs := make(chan worker.Job, 200)
	results := worker.StartWorkerPool(ctx, jobs)

	// Walk the filesystem in a goroutine, communicate errors via channel.
	walkErr := make(chan error, 1)
	go func() {
		walkErr <- walker.Walk(ctx, path, jobs)
	}()

	// Aggregate blocks until the results channel is closed (all workers done)
	// or the context is cancelled.
	aggregatedFindings := aggregator.Aggregate(ctx, results)

	// Wait for the walker to finish and check for errors.
	// The walker always sends to this buffered channel before its goroutine exits,
	// so this read will not block indefinitely.
	if err := <-walkErr; err != nil {
		log.Printf("Warning: walker encountered errors (partial results may be returned): %v", err)
	}

	output.PrintFindings(aggregatedFindings, path)
	output.SaveReport(aggregatedFindings, path, outputFile, format)
	return nil
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a repository for secrets and APIs",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return runScan(ctx)
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
