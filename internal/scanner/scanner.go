package scanner

import (
	"bufio"
	"os"
	"strings"

	"github.com/Dharshan2208/git-scanner/internal/detector"
	"github.com/Dharshan2208/git-scanner/internal/types"
)

func scanLines(s *bufio.Scanner, filePath string, commit string, message string) []types.Finding {
	var findings []types.Finding

	// default token limit is small; bump to handle long lines (minified json, JWTs, etc.)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 1
	for s.Scan() {
		line := s.Text()

		// 1.Signature based detection
		for _, sig := range detector.Signatures {
			if sig.Regex.MatchString(line) {
				findings = append(findings, types.Finding{
					File:    filePath,
					Line:    lineNum,
					Type:    sig.Name,
					Match:   line,
					Commit:  commit,
					Message: message,
				})
			}
		}

		// 2.Entropy based detection
		entropyMatches := detector.FindHighEntropy(line)

		for _, match := range entropyMatches {
			findings = append(findings, types.Finding{
				File:    filePath,
				Line:    lineNum,
				Type:    "High Entropy String",
				Match:   match,
				Commit:  commit,
				Message: message,
			})
		}

		lineNum++
	}

	return findings
}

// scans a file and returns findings
func ScanFile(filePath string, commit string, message string) []types.Finding {
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	s := bufio.NewScanner(file)
	return scanLines(s, filePath, commit, message)
}

// ScanContent scans in-memory content (used for git history trees where files are not checked out).
func ScanContent(content string, virtualPath string, commit string, message string) []types.Finding {
	s := bufio.NewScanner(strings.NewReader(content))
	return scanLines(s, virtualPath, commit, message)
}
