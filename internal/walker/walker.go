package walker

import (
	"io/fs"
	"path/filepath"
	"strings"
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
func Walk(root string, jobs chan<- string) error {
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
			select {
			case jobs <- path:
			default:
				// Channel is full or closed (rare), but safe
			}
		}

		return nil
	})
}
