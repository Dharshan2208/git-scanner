package lifecycle

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/Dharshan2208/git-scanner/internal/aggregator"
	"github.com/Dharshan2208/git-scanner/internal/git"
	"github.com/Dharshan2208/git-scanner/internal/scanner"
	"github.com/Dharshan2208/git-scanner/internal/types"
	"github.com/Dharshan2208/git-scanner/internal/utils"
	"github.com/Dharshan2208/git-scanner/internal/walker"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitTreeScanner struct {
	BasePath string
}

// ScanCommit scans every file in the given commit tree and returns findings.
func (s *CommitTreeScanner) ScanCommit(ctx context.Context, info git.CommitInfo, tree *object.Tree) []types.Finding {
	jobs, err := walker.CollectJobsFromTree(ctx, tree, s.BasePath)
	if err != nil {
		log.Printf("Warning: failed to collect jobs for commit %s: %v",
			utils.ShortHash(info.Hash, 8), err)
		return nil
	}

	var findings []types.Finding
	for _, job := range jobs {
		results := scanner.ScanContent(ctx, job.Content, job.FilePath, info.Hash, info.Message)
		findings = append(findings, results...)
	}
	return findings
}

// RunParallelHistoryScan performs a full parallel git history scan on the
// repository at repoPath, then enriches findings with lifecycle tracking.
// Returns the enriched findings or an error.
func RunParallelHistoryScan(ctx context.Context, repoPath string) ([]types.Finding, error) {
	fmt.Println("Starting PARALLEL git history scan...")
	fmt.Println("(Scanning multiple commits concurrently for faster results)")
	fmt.Println()

	treeScanner := &CommitTreeScanner{BasePath: repoPath}

	findings, err := git.ScanHistoryParallel(ctx, repoPath, treeScanner)
	if err != nil {
		return nil, fmt.Errorf("parallel history scan failed: %w", err)
	}

	fmt.Println("\nBuilding lifecycle tracking...")
	return BuildLifecycle(ctx, findings, repoPath), nil
}

// BuildLifecycle processes findings in commit order (oldest → newest) to
// determine each secret's lifecycle: when it was introduced, removed, how
// many commits it was exposed in, and whether it still exists in HEAD.
//
// It returns a new slice of findings enriched with lifecycle metadata.
func BuildLifecycle(ctx context.Context, findings []types.Finding, repoPath string) []types.Finding {
	type lifecycle struct {
		IntroducedCommit string
		RemovedCommit    string
		ExposureCommits  int
		Active           bool
	}

	secretKey := func(f types.Finding) string {
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
			short := utils.ShortHash(removedCommit, 8)
			return fmt.Sprintf("Exposed for %d %s (removed in commit %s)", exposureCommits, commitWord, short)
		}
		return fmt.Sprintf("Exposed for %d %s", exposureCommits, commitWord)
	}

	// Group findings by commit
	commitsByHash := make(map[string][]types.Finding)
	for _, f := range findings {
		commitsByHash[f.Commit] = append(commitsByHash[f.Commit], f)
	}

	// Get commit order (oldest → newest)
	commitOrder, err := git.GetCommitOrder(ctx, repoPath)
	if err != nil {
		log.Printf("Warning: could not get commit order, using hash order: %v", err)
	}

	// Sort commit hashes by order index
	var orderedCommits []commitEntry
	for hash := range commitsByHash {
		order := 0
		if commitOrder != nil {
			if o, ok := commitOrder[hash]; ok {
				order = o
			}
		}
		orderedCommits = append(orderedCommits, commitEntry{hash: hash, order: order})
	}
	sortCommitsByOrder(orderedCommits)

	// Process commits oldest → newest to build lifecycle state machine
	lifecycles := make(map[string]*lifecycle)
	activeKeys := make(map[string]struct{})

	for _, co := range orderedCommits {
		// Check for cancellation during lifecycle processing.
		if err := ctx.Err(); err != nil {
			break
		}

		commitFindings := commitsByHash[co.hash]

		presentKeys := make(map[string]struct{})
		for _, f := range commitFindings {
			presentKeys[secretKey(f)] = struct{}{}
		}

		// Mark removals: secrets that existed in previous commit but not in this one
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

		// Mark introductions and count exposures
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

	// Enrich findings with lifecycle info (deduplicated)
	var enriched []types.Finding
	seen := make(map[string]bool)

	for _, f := range findings {
		key := f.File + "|" + fmt.Sprintf("%d", f.Line) + "|" + f.Type + "|" + f.Match
		if seen[key] {
			continue
		}
		seen[key] = true

		lck := secretKey(f)
		if lc, ok := lifecycles[lck]; ok {
			f.IntroducedCommit = lc.IntroducedCommit
			f.ExposureCommits = lc.ExposureCommits
			f.StillPresentInHEAD = lc.Active
			if !lc.Active {
				f.RemovedCommit = lc.RemovedCommit
			}
			f.ExposureWindow = formatExposure(lc.ExposureCommits, lc.Active, lc.RemovedCommit)
			f.ExposureWindow = strings.ReplaceAll(f.ExposureWindow, "\n", " ")
		}

		enriched = append(enriched, f)
	}

	aggregator.SortFindings(enriched)
	return enriched
}

// commitEntry pairs a commit hash with its order index for sorting.
type commitEntry struct {
	hash  string
	order int
}

func sortCommitsByOrder(commits []commitEntry) {
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].order < commits[j].order
	})
}
