package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
)

// Resolve will take the input (local/repo url)
// and returns a local directory path
func Resolve(input string) (string, func() error, error) {
	// Case 1: Remote repo
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {

		// Get the current working directory
		projectDir, err := os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("failed to get current directory: %w", err)
		}

		// Create "temp" folder inside project if it doesn't exist
		tempBase := filepath.Join(projectDir, "temp")
		if err := os.MkdirAll(tempBase, 0o755); err != nil {
			return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
		}

		// Creating unique folder inside temp directory
		tempDir, err := os.MkdirTemp(tempBase, "git-scanner-*")
		if err != nil {
			return "", nil, err
		}

		fmt.Println("Cloning repo into:", tempDir)

		// Clone repo
		_, err = git.PlainClone(tempDir, false, &git.CloneOptions{
			URL:      input,
			Progress: os.Stdout,
		})
		if err != nil {
			os.RemoveAll(tempDir) // cleanup on clone failure
			return "", nil, err
		}

		// Cleanup function
		cleanup := func() error {
			fmt.Println("Cleaning up the temp directory : ", tempDir)
			return os.RemoveAll(tempDir)
		}

		return tempDir, cleanup, nil
	}

	// Case 2: Local path
	if _, err := os.Stat(input); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("local path does not exist: %s", input)
	}

	// No cleanup needed for local paths
	return input, func() error { return nil }, nil
}
