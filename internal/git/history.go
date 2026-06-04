package git

import (
	"fmt"
	"log"
	"runtime"
	"sync"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

// CommitInfo contains metadata about a commit
type CommitInfo struct {
	Hash       string
	Message    string
	OrderIndex int // Position in history (0 = oldest, for lifecycle tracking)
}

// shortHash safely truncates a commit hash to a readable length.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// ScanHistory passes the commit's tree so walker can scan without checkout.
// This is the original sequential implementation — kept for reference.
func ScanHistory(repoPath string, callback func(CommitInfo, *object.Tree)) error {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	iter, err := r.Log(&git.LogOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to get commit log: %w", err)
	}

	var commits []*object.Commit
	if err := iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	}); err != nil {
		return err
	}

	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		tree, err := c.Tree()
		if err != nil {
			log.Printf("Warning: failed to get tree for commit %s: %v", shortHash(c.Hash.String()), err)
			continue
		}

		fmt.Printf("Scanning commit %s | %s\n", shortHash(c.Hash.String()), truncate(c.Message, 70))

		callback(CommitInfo{
			Hash:       c.Hash.String(),
			Message:    c.Message,
			OrderIndex: len(commits) - 1 - i,
		}, tree)
	}

	return nil
}

// CommitScanner is implemented by types that know how to scan a single commit's tree.
type CommitScanner interface {
	ScanCommit(info CommitInfo, tree *object.Tree) []types.Finding
}

// ScanHistoryParallel scans every commit in the repository in parallel using the
// provided CommitScanner and returns all findings across all commits.
func ScanHistoryParallel(repoPath string, scanner CommitScanner) ([]types.Finding, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	iter, err := r.Log(&git.LogOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}

	// Collect all commits (newest first from go-git)
	var commits []*object.Commit
	if err := iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	}); err != nil {
		return nil, err
	}

	// Reverse to get oldest first for proper lifecycle tracking
	numCommits := len(commits)

	type scanTask struct {
		commit *object.Commit
		tree   *object.Tree
		info   CommitInfo
	}

	tasks := make([]scanTask, 0, numCommits)
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		tree, err := c.Tree()
		if err != nil {
			log.Printf("Warning: failed to get tree for commit %s: %v", shortHash(c.Hash.String()), err)
			continue
		}

		tasks = append(tasks, scanTask{
			commit: c,
			tree:   tree,
			info: CommitInfo{
				Hash:       c.Hash.String(),
				Message:    c.Message,
				OrderIndex: numCommits - 1 - i,
			},
		})
	}

	log.Printf("Found %d commits, scanning in parallel with %d workers...\n", len(tasks), runtime.NumCPU())

	type result struct {
		info     CommitInfo
		findings []types.Finding
	}

	resultsChan := make(chan result, len(tasks))
	taskChan := make(chan scanTask, len(tasks))
	var wg sync.WaitGroup

	// Start worker pool
	numWorkers := runtime.NumCPU()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				fmt.Printf("[Worker %d] Scanning commit %s | %s\n",
					workerID+1, shortHash(task.info.Hash), truncate(task.info.Message, 50))

				findings := scanner.ScanCommit(task.info, task.tree)

				// Stamp commit info on each finding
				for i := range findings {
					findings[i].Commit = task.info.Hash
					findings[i].Message = task.info.Message
				}

				resultsChan <- result{
					info:     task.info,
					findings: findings,
				}
			}
		}(w)
	}

	// Feed tasks to workers
	go func() {
		for _, task := range tasks {
			taskChan <- task
		}
		close(taskChan)
	}()

	// Close results when done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results
	var allFindings []types.Finding
	for res := range resultsChan {
		allFindings = append(allFindings, res.findings...)
	}

	log.Printf("Parallel scan complete: %d total findings from %d commits\n", len(allFindings), len(tasks))

	return allFindings, nil
}

// GetCommitOrder returns a map of commit hash to order index (oldest = 0).
func GetCommitOrder(repoPath string) (map[string]int, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	iter, err := r.Log(&git.LogOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}

	var commits []*object.Commit
	if err := iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	}); err != nil {
		return nil, err
	}

	orderMap := make(map[string]int)
	numCommits := len(commits)

	for i := len(commits) - 1; i >= 0; i-- {
		orderMap[commits[i].Hash.String()] = numCommits - 1 - i
	}

	return orderMap, nil
}

// GetCommitInfo returns detailed info for a specific commit hash.
func GetCommitInfo(repoPath, commitHash string) (*CommitInfo, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := r.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", shortHash(commitHash), err)
	}

	orderMap, err := GetCommitOrder(repoPath)
	if err != nil {
		return nil, err
	}

	return &CommitInfo{
		Hash:       commit.Hash.String(),
		Message:    commit.Message,
		OrderIndex: orderMap[commit.Hash.String()],
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
