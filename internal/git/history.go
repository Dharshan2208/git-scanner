package git

import (
	"fmt"
	"log"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Hash    string
	Message string
}

// ScanHistory passes the commit's tree so walker can scan without checkout
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
			Hash:    c.Hash.String(),
			Message: c.Message,
		}, tree)
	}

	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
