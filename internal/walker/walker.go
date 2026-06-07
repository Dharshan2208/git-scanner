package walker

import (
	"context"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/Dharshan2208/git-scanner/internal/worker"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// extensions we care about
var ValidExt = map[string]bool{
	".go":   true,
	".js":   true,
	".ts":   true,
	".py":   true,
	".env":  true,
	".json": true,
	".yaml": true,
	".yml":  true,
	".txt":  true,
	".c":    true,
}

// directories to skip
var SkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
}

// files to skip even though they have proper valid extension
var SkipFiles = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"go.sum":            true,
	"composer.lock":     true,
	".DS_Store":         true,
}

// Walk traverses the directory and sends valid file paths to jobs channel.
// It respects ctx cancellation: when ctx is cancelled it stops the walk
// and closes the jobs channel.
func Walk(ctx context.Context, root string, jobs chan<- worker.Job) error {
	defer close(jobs)

	// Wrap filepath.WalkDir so we can bail on cancellation.
	var walkErr error
	done := make(chan struct{}, 1)

	go func() {
		walkErr = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			// Check cancellation on every directory entry.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			if err != nil {
				return err
			}

			if d.IsDir() {
				if SkipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}

			if SkipFiles[d.Name()] {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ValidExt[ext] {
				select {
				case jobs <- worker.Job{
					FilePath: path,
					Commit:   "",
					Message:  "",
				}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		// Cancellation occurred mid-walk. Return the context error;
		// the goroutine will finish and close jobs naturally.
		return ctx.Err()
	case <-done:
		return walkErr
	}
}

// CollectJobsFromTree collects all jobs from a git tree and returns them.
func CollectJobsFromTree(ctx context.Context, tree *object.Tree, basePath string) ([]worker.Job, error) {
	var jobs []worker.Job
	err := walkGitTreeWithCollector(ctx, tree, basePath, func(job worker.Job) {
		jobs = append(jobs, job)
	})
	return jobs, err
}

// --- Internal implementation ---
func walkGitTreeWithCollector(ctx context.Context, tree *object.Tree, basePath string, collector func(worker.Job)) error {
	return tree.Files().ForEach(func(f *object.File) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		job, ok := createJobFromGitFile(f, basePath)
		if !ok {
			return nil
		}
		collector(job)
		return nil
	})
}

// createJobFromGitFile creates a job from a git file if it passes filters
// Returns (job, true) if valid, (empty, false) if should be skipped
func createJobFromGitFile(f *object.File, basePath string) (worker.Job, bool) {
	// Skip directories anywhere in the path
	for _, part := range strings.Split(f.Name, "/") {
		if SkipDirs[part] {
			return worker.Job{}, false
		}
	}

	baseName := filepath.Base(f.Name)
	if SkipFiles[baseName] {
		return worker.Job{}, false
	}

	ext := strings.ToLower(filepath.Ext(f.Name))
	if !ValidExt[ext] {
		return worker.Job{}, false
	}

	// Skip very large files
	if f.Size > 500*1024 {
		return worker.Job{}, false
	}

	// Get file content
	content, err := f.Contents()
	if err != nil {
		log.Printf("Warning: failed to read %s: %v", f.Name, err)
		return worker.Job{}, false
	}

	fullPath := filepath.Join(basePath, f.Name)

	return worker.Job{
		FilePath: fullPath,
		Content:  content,
		Commit:   "",
		Message:  "",
	}, true
}
