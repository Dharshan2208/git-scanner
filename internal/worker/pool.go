package worker

import (
	"context"
	"runtime"
	"sync"

	"github.com/Dharshan2208/git-scanner/internal/scanner"
	"github.com/Dharshan2208/git-scanner/internal/types"
)

type Finding = types.Finding

type Job struct {
	FilePath string
	Content  string
	Commit   string
	Message  string
}

// StartWorkerPool starts workers and returns results channel.
func StartWorkerPool(ctx context.Context, jobs chan Job) chan Finding {
	results := make(chan Finding)
	var wg sync.WaitGroup

	// number of workers = CPU cores
	numWorkers := runtime.NumCPU()

	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go worker(ctx, jobs, results, &wg)
	}

	// close results after all workers finish
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// worker processes files until the context is cancelled or the jobs channel is closed.
func worker(ctx context.Context, jobs chan Job, results chan Finding, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled; drain remaining jobs without processing them
			// so the walker goroutine doesn't block.
			for range jobs {
				// drain
			}
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			var findings []types.Finding
			if job.Content != "" {
				findings = scanner.ScanContent(ctx, job.Content, job.FilePath, job.Commit, job.Message)
			} else {
				findings = scanner.ScanFile(ctx, job.FilePath, job.Commit, job.Message)
			}

			for _, f := range findings {
				if job.Commit != "" {
					f.Commit = job.Commit
					f.Message = job.Message
				}
				results <- f
			}
		}
	}
}
