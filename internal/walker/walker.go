package walker

import (
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/Dharshan2208/git-scanner/internal/worker"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// extensions we care about
var validExt = map[string]bool{
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
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
}

// files to skip even though they have proper valid extension
var skipFiles = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"go.sum":            true,
	"composer.lock":     true,
	".DS_Store":         true,
}

// Walk traverses the directory and sends valid file paths to jobs channel
func Walk(root string, jobs chan<- worker.Job) error {
	// clsing the channels only when we are completely done
	defer close(jobs)

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		if skipFiles[d.Name()] {
			return nil
		}

		// only process the files with valid extension
		ext := strings.ToLower(filepath.Ext(path))
		if validExt[ext] {
			job := worker.Job{
				FilePath: path,
				Commit:   "",
				Message:  "",
			}

			jobs <- job
		}

		return nil
	})
}

func WalkTree(tree *object.Tree, basePath string, jobs chan<- worker.Job) error {
	return tree.Files().ForEach(func(f *object.File) error {
		// Skip directories anywhere in the path
		for _, part := range strings.Split(f.Name, "/") {
			if skipDirs[part] {
				return nil
			}
		}

		baseName := filepath.Base(f.Name)
		if skipFiles[baseName] {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(f.Name))
		if !validExt[ext] {
			return nil
		}

		// Skip very large files (optional but recommended)
		if f.Size > 500*1024 {
			return nil
		}

		// Get file content
		content, err := f.Contents()
		if err != nil {
			log.Printf("Warning: failed to read %s: %v", f.Name, err)
			return nil
		}

		// Full path for reporting (you can change this if you want just relative name)
		fullPath := filepath.Join(basePath, f.Name)

		jobs <- worker.Job{
			FilePath: fullPath,
			Content:  content,
			Commit:   "",
			Message:  "",
		}

		return nil
	})
}
