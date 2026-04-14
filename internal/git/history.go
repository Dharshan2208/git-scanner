package git

import (
	"fmt"
	"log"
	"runtime"
	"sync"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CommitInfo contains metadata about a commit
type CommitInfo struct {
	Hash       string
	Message    string
	OrderIndex int // Position in history (0 = oldest, for lifecycle tracking)
}

// ScanResult holds the findings for a single commit
type ScanResult struct {
	Commit CommitInfo
	Tree   *object.Tree
}

// ScanHistory passes the commit's tree so walker can scan without checkout
// This is the original sequential implementation - kept for compatibility
func ScanHistory(repoPath string, callback func(CommitInfo, *object.Tree)) error {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	iter, err := r.Log(&git.LogOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to get commit log: %w", err)
	}

	// go-git returns commits from newest -> oldest. Lifecycle tracking and exposure
	// windows are easier to compute when scanning oldest -> newest, so we buffer
	// commits and then iterate in reverse.
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
			log.Printf("Warning: failed to get tree for commit %s: %v", c.Hash.String()[:8], err)
			continue
		}

		fmt.Printf("Scanning commit %s | %s\n", c.Hash.String()[:8], truncate(c.Message, 70))

		callback(CommitInfo{
			Hash:       c.Hash.String(),
			Message:    c.Message,
			OrderIndex: len(commits) - 1 - i,
		}, tree)
	}

	return nil
}

type CommitScanner interface {
	ScanCommit(info CommitInfo, tree *object.Tree) []Finding
}

type Finding struct {
	File    string
	Line    int
	Type    string
	Match   string
	Commit  string
	Message string
}

func ScanHistoryParallel(repoPath string, scanner CommitScanner) ([]Finding, error) {
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
	// But we need to remember the original order for lifecycle
	numCommits := len(commits)

	// Create scan tasks (oldest to newest for lifecycle)
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
			log.Printf("Warning: failed to get tree for commit %s: %v", c.Hash.String()[:8], err)
			continue
		}

		tasks = append(tasks, scanTask{
			commit: c,
			tree:   tree,
			info: CommitInfo{
				Hash:       c.Hash.String(),
				Message:    c.Message,
				OrderIndex: numCommits - 1 - i, // Oldest = 0, newest = N-1
			},
		})
	}

	log.Printf("Found %d commits, scanning in parallel with %d workers...\n", len(tasks), runtime.NumCPU())

	// Results channel and waitgroup for parallel processing
	type result struct {
		info     CommitInfo
		findings []Finding
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
					workerID+1, task.info.Hash[:8], truncate(task.info.Message, 50))

				findings := scanner.ScanCommit(task.info, task.tree)

				// Add commit info to each finding
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
	var allFindings []Finding
	for res := range resultsChan {
		allFindings = append(allFindings, res.findings...)
	}

	log.Printf("Parallel scan complete: %d total findings from %d commits\n", len(allFindings), len(tasks))

	return allFindings, nil
}

// GetCommitOrder returns a map of commit hash to order index (oldest = 0)
// This is useful for lifecycle tracking after parallel scanning
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

	// Reverse iterate (oldest to newest)
	for i := len(commits) - 1; i >= 0; i-- {
		orderMap[commits[i].Hash.String()] = numCommits - 1 - i
	}

	return orderMap, nil
}

// GetCommitInfo returns detailed info for a specific commit hash
func GetCommitInfo(repoPath, commitHash string) (*CommitInfo, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := r.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", commitHash[:8], err)
	}

	// Get order index
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
