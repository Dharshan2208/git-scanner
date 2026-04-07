package worker

import (
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

// StartWorkerPool starts workers and returns results channel
func StartWorkerPool(jobs chan Job) chan Finding {
	results := make(chan Finding)
	var wg sync.WaitGroup

	// number of workers = CPU cores
	numWorkers := runtime.NumCPU()

	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go worker(jobs, results, &wg)
	}

	// close results after all workers finish
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// worker processes files
func worker(jobs chan Job, results chan Finding, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		var findings []types.Finding
		if job.Content != "" {
			findings = scanner.ScanContent(job.Content, job.FilePath, job.Commit, job.Message)
		} else {
			findings = scanner.ScanFile(job.FilePath, job.Commit, job.Message)
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
