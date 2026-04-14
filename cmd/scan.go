package cmd

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sort"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/git"
	"github.com/Dharshan2208/git-scanner/internal/output"
	"github.com/Dharshan2208/git-scanner/internal/repo"
	"github.com/Dharshan2208/git-scanner/internal/scanner"
	"github.com/Dharshan2208/git-scanner/internal/utils"
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
			runHistoryScanParallel(path)
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

		// Aggregate findings (deduplicate + sort)
		aggregatedFindings := aggregator.Aggregate(results)
		printFindings(aggregatedFindings, path)
		saveReport(aggregatedFindings, path)
	},
}

// CommitTreeScanner implements git.CommitScanner for parallel history scanning
// Uses walker package for file traversal logic
type CommitTreeScanner struct {
	BasePath string
}

func (s *CommitTreeScanner) ScanCommit(info git.CommitInfo, tree *object.Tree) []git.Finding {
	var findings []git.Finding

	// Collect jobs from the git tree using walker
	jobs, err := walker.CollectJobsFromTree(tree, s.BasePath)
	if err != nil {
		log.Printf("Warning: failed to collect jobs for commit %s: %v", info.Hash[:8], err)
		return findings
	}

	// Process each job (scan the content)
	for _, job := range jobs {
		results := scanner.ScanContent(job.Content, job.FilePath, info.Hash, info.Message)

		// Convert to git.Finding
		for _, r := range results {
			findings = append(findings, git.Finding{
				File:    r.File,
				Line:    r.Line,
				Type:    r.Type,
				Match:   r.Match,
				Commit:  info.Hash,
				Message: info.Message,
			})
		}
	}

	return findings
}

// Parallel history scan - scans commits concurrently for better performance
func runHistoryScanParallel(repoPath string) {
	fmt.Println("Starting PARALLEL git history scan...")
	fmt.Println("(Scanning multiple commits concurrently for faster results)")
	fmt.Println()

	// Create scanner
	treeScanner := &CommitTreeScanner{BasePath: repoPath}

	// Scan all commits in parallel
	findings, err := git.ScanHistoryParallel(repoPath, treeScanner)
	if err != nil {
		log.Fatal("Parallel history scan failed:", err)
	}

	fmt.Println("\nBuilding lifecycle tracking...")
	buildLifecycle(findings, repoPath)
}

// Sequential lifecycle building - processes findings in commit order
func buildLifecycle(findings []git.Finding, repoPath string) {
	type lifecycle struct {
		IntroducedCommit string
		RemovedCommit    string
		ExposureCommits  int
		Active           bool
	}

	secretKey := func(f git.Finding) string {
		return f.Type + "\x00" + f.Match
	}

	formatExposure := func(exposureCommits int, stillPresent bool, removedCommit string) string {
		if exposureCommits <= 0 {
			return ""
		}

		commitWord := "commits"
		if exposureCommits == 1 {
			commitWord = "commit"
		}

		if stillPresent {
			return fmt.Sprintf("Exposed for %d %s (still present in HEAD)", exposureCommits, commitWord)
		}
		if removedCommit != "" {
			short := removedCommit
			if len(short) > 8 {
				short = short[:8]
			}
			return fmt.Sprintf("Exposed for %d %s (removed in commit %s)", exposureCommits, commitWord, short)
		}
		return fmt.Sprintf("Exposed for %d %s", exposureCommits, commitWord)
	}

	// Group findings by commit for ordered processing
	commitsByHash := make(map[string][]git.Finding)
	for _, f := range findings {
		commitsByHash[f.Commit] = append(commitsByHash[f.Commit], f)
	}

	// Get commit order (oldest to newest)
	commitOrder, err := git.GetCommitOrder(repoPath)
	if err != nil {
		log.Printf("Warning: could not get commit order, using hash order: %v", err)
	}

	// Sort commit hashes by order
	var orderedCommits []commitEntry
	for hash := range commitsByHash {
		order := commitOrder[hash]
		orderedCommits = append(orderedCommits, commitEntry{hash: hash, order: order})
	}

	// Sort by order index (oldest first)
	sortCommitsByOrder(orderedCommits)

	// Process in order to build lifecycle
	lifecycles := make(map[string]*lifecycle)
	activeKeys := make(map[string]struct{})

	for _, co := range orderedCommits {
		commitFindings := commitsByHash[co.hash]

		// Build present keys set for this commit
		presentKeys := make(map[string]struct{})
		for _, f := range commitFindings {
			presentKeys[secretKey(f)] = struct{}{}
		}

		// 1) Mark removals: secrets in previous commit but not in current
		for key := range activeKeys {
			if _, ok := presentKeys[key]; ok {
				continue
			}
			if lc, ok := lifecycles[key]; ok && lc.Active {
				lc.Active = false
				lc.RemovedCommit = co.hash
			}
			delete(activeKeys, key)
		}

		// 2) Mark introductions and exposures
		for key := range presentKeys {
			lc, ok := lifecycles[key]
			if !ok {
				lc = &lifecycle{IntroducedCommit: co.hash}
				lifecycles[key] = lc
			}
			lc.Active = true
			lc.ExposureCommits++
			activeKeys[key] = struct{}{}
		}
	}

	// Convert findings to worker.Finding and apply lifecycle info
	var allFindings []worker.Finding
	seen := make(map[string]bool)

	for _, f := range findings {
		key := f.File + "|" + fmt.Sprintf("%d", f.Line) + "|" + f.Type + "|" + f.Match
		if seen[key] {
			continue
		}
		seen[key] = true

		wf := worker.Finding{
			File:    f.File,
			Line:    f.Line,
			Type:    f.Type,
			Match:   f.Match,
			Commit:  f.Commit,
			Message: f.Message,
		}

		// Apply lifecycle info
		lck := secretKey(f)
		if lc, ok := lifecycles[lck]; ok {
			wf.IntroducedCommit = lc.IntroducedCommit
			wf.ExposureCommits = lc.ExposureCommits
			wf.StillPresentInHEAD = lc.Active
			if !lc.Active {
				wf.RemovedCommit = lc.RemovedCommit
			}
			wf.ExposureWindow = formatExposure(lc.ExposureCommits, lc.Active, lc.RemovedCommit)
			wf.ExposureWindow = strings.ReplaceAll(wf.ExposureWindow, "\n", " ")
		}

		allFindings = append(allFindings, wf)
	}

	sortFindings(allFindings)

	fmt.Println("\nHistory scan completed.")
	printFindings(allFindings, repoPath)
	saveReport(allFindings, repoPath)
}

// commitEntry is a helper struct for sorting
type commitEntry struct {
	hash  string
	order int
}

func sortCommitsByOrder(commits []commitEntry) {
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].order < commits[j].order
	})
}

func sortFindings(findings []worker.Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File == findings[j].File {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].File < findings[j].File
	})
}

// Old full hisrtory scanner without parallel scanning
// func runHistoryScan(repoPath string) {
// 	fmt.Println("Starting full git history scan... This may take a while.")

// 	var allFindings []worker.Finding

// 	type lifecycle struct {
// 		IntroducedCommit string
// 		RemovedCommit    string
// 		ExposureCommits  int
// 		Active           bool
// 	}

// 	secretKey := func(f worker.Finding) string {
// 		return f.Type + "\x00" + f.Match
// 	}

// 	formatExposure := func(exposureCommits int, stillPresent bool, removedCommit string) string {
// 		if exposureCommits <= 0 {
// 			return ""
// 		}

// 		commitWord := "commits"
// 		if exposureCommits == 1 {
// 			commitWord = "commit"
// 		}

// 		if stillPresent {
// 			return fmt.Sprintf("Exposed for %d %s (still present in HEAD)", exposureCommits, commitWord)
// 		}
// 		if removedCommit != "" {
// 			short := removedCommit
// 			if len(short) > 8 {
// 				short = short[:8]
// 			}
// 			return fmt.Sprintf("Exposed for %d %s (removed in commit %s)", exposureCommits, commitWord, short)
// 		}
// 		return fmt.Sprintf("Exposed for %d %s", exposureCommits, commitWord)
// 	}

// 	lifecycles := make(map[string]*lifecycle)
// 	activeKeys := make(map[string]struct{})

// 	err := git.ScanHistory(repoPath, func(commitInfo git.CommitInfo, tree *object.Tree) {
// 		// Create fresh channels for this commit
// 		jobs := make(chan worker.Job, 200)
// 		results := worker.StartWorkerPool(jobs)

// 		// Walk the *tree* using walker package
// 		go func() {
// 			if err := walker.WalkTree(tree, repoPath, jobs); err != nil {
// 				log.Printf("Warning: walker failed for commit %s: %v", commitInfo.Hash[:8], err)
// 			}
// 			close(jobs)
// 		}()

// 		commitFindings := aggregator.Aggregate(results)

// 		// Attach commit info
// 		for i := range commitFindings {
// 			commitFindings[i].Commit = commitInfo.Hash
// 			commitFindings[i].Message = commitInfo.Message
// 		}

// 		// Build per-commit secret set (unique by Type+Match) for lifecycle updates.
// 		presentKeys := make(map[string]struct{})
// 		for _, f := range commitFindings {
// 			presentKeys[secretKey(f)] = struct{}{}
// 		}

// 		// 1) Mark removals: any secret active in the previous commit but missing now is removed in this commit.
// 		for key := range activeKeys {
// 			if _, ok := presentKeys[key]; ok {
// 				continue
// 			}
// 			if lc, ok := lifecycles[key]; ok && lc.Active {
// 				lc.Active = false
// 				lc.RemovedCommit = commitInfo.Hash
// 			}
// 			delete(activeKeys, key)
// 		}

// 		// 2) Mark presence/exposure and introductions.
// 		for key := range presentKeys {
// 			lc, ok := lifecycles[key]
// 			if !ok {
// 				lc = &lifecycle{IntroducedCommit: commitInfo.Hash}
// 				lifecycles[key] = lc
// 			}
// 			lc.Active = true
// 			lc.ExposureCommits++
// 			activeKeys[key] = struct{}{}
// 		}

// 		allFindings = append(allFindings, commitFindings...)
// 	})
// 	if err != nil {
// 		log.Fatal("History scan failed:", err)
// 	}

// 	for i := range allFindings {
// 		key := secretKey(allFindings[i])
// 		lc := lifecycles[key]
// 		if lc == nil {
// 			continue
// 		}

// 		allFindings[i].IntroducedCommit = lc.IntroducedCommit
// 		allFindings[i].ExposureCommits = lc.ExposureCommits
// 		allFindings[i].StillPresentInHEAD = lc.Active

// 		// Only set RemovedCommit if the secret is not present in HEAD.
// 		if !lc.Active {
// 			allFindings[i].RemovedCommit = lc.RemovedCommit
// 		}
// 		allFindings[i].ExposureWindow = formatExposure(lc.ExposureCommits, lc.Active, lc.RemovedCommit)

// 		allFindings[i].ExposureWindow = strings.ReplaceAll(allFindings[i].ExposureWindow, "\n", " ")
// 	}

// 	fmt.Println("\nHistory scan completed.")
// 	printFindings(allFindings, repoPath)
// 	saveReport(allFindings, repoPath)
// }

// Helper functions to reduce congestion in the above code
func printFindings(findings []worker.Finding, basePath string) {
	foundCount := 0
	for _, f := range findings {
		relPath, _ := filepath.Rel(basePath, f.File)
		sanitizedMatch := utils.SanitizeSecret(f.Match)
		fmt.Printf("[FOUND] %s | %s | Line : %d\n", relPath, f.Type, f.Line)
		fmt.Printf("       Match: %s\n", sanitizedMatch)
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
