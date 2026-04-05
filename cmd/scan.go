package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var localPath string
var repoURL string

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a repository for secrets and APIs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Scanning...")
		fmt.Println("Local:", localPath)
		fmt.Println("Repo:", repoURL)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&localPath, "local", "", "Local directory to scan")
	scanCmd.Flags().StringVar(&repoURL, "repo", "", "Git repository URL")
}