package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Dharshan2208/git-scanner/internal/types"
)

// WriteJSON writes findings into a JSON report
func WriteJSON(findings []types.Finding, basePath, outputPath string) error {
	type ReportFinding struct {
		File    string `json:"file"`
		Type    string `json:"type"`
		Line    int    `json:"line"`
		Match   string `json:"match"`
		Commit  string `json:"commit,omitempty"`
		Message string `json:"message,omitempty"`

		// Lifecycle tracking (populated during --history scans)
		IntroducedCommit   string `json:"introduced_commit,omitempty"`
		RemovedCommit      string `json:"removed_commit,omitempty"`
		ExposureCommits    int    `json:"exposure_commits,omitempty"`
		ExposureWindow     string `json:"exposure_window,omitempty"`
		StillPresentInHEAD bool   `json:"still_present_in_head,omitempty"`
	}

	report := struct {
		Repository    string          `json:"repository,omitempty"`
		ScanTime      string          `json:"scan_time"`
		TotalFindings int             `json:"total_findings"`
		Findings      []ReportFinding `json:"findings"`
	}{
		ScanTime:      getCurrentTime(),
		TotalFindings: len(findings),
		Findings:      make([]ReportFinding, len(findings)),
	}

	for i, f := range findings {
		relPath, err := filepath.Rel(basePath, f.File)
		if err != nil {
			relPath = f.File
		}

		report.Findings[i] = ReportFinding{
			File:    relPath,
			Type:    f.Type,
			Line:    f.Line,
			Match:   f.Match,
			Commit:  f.Commit,
			Message: f.Message,

			IntroducedCommit:   f.IntroducedCommit,
			RemovedCommit:      f.RemovedCommit,
			ExposureCommits:    f.ExposureCommits,
			ExposureWindow:     f.ExposureWindow,
			StillPresentInHEAD: f.StillPresentInHEAD,
		}
	}

	// Pretty print JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}

	return nil
}

func getCurrentTime() string {
	return time.Now().Format(time.RFC3339)
}
