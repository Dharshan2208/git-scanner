package worker

import (
	"runtime"
	"sync"
)

// detected issue structure
type Finding struct {
	File     string
	Line     int
	Type     string
	Severity string
	Match    string
}

// StartWorkerPool starts workers and returns results channel
func StartWorkerPool(jobs chan string) chan Finding {
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
func worker(jobs chan string, results chan Finding, wg *sync.WaitGroup) {
	defer wg.Done()

	for file := range jobs {
		// fake processing
		results <- Finding{
			File:     file,
			Line:     1,
			Type:     "demo",
			Severity: "LOW",
			Match:    "example",
		}
	}
}
