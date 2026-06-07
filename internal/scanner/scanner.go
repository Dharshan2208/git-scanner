package scanner

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/Dharshan2208/git-scanner/internal/detector"
	"github.com/Dharshan2208/git-scanner/internal/types"
)

func scanLines(ctx context.Context, s *bufio.Scanner, filePath string, commit string, message string) []types.Finding {
	var findings []types.Finding

	// default token limit is small; bump to handle long lines (minified json, JWTs, etc.)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 1
	for s.Scan() {
		// Check for cancellation between lines to avoid long-running scans of huge files.
		if err := ctx.Err(); err != nil {
			return findings
		}

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

		// 2. Keyword based detection
		keywordMatches := detector.FindKeywordBased(line)
		for _, match := range keywordMatches {
			findings = append(findings, types.Finding{
				File:    filePath,
				Line:    lineNum,
				Type:    "Keyword Secret",
				Match:   match,
				Commit:  commit,
				Message: message,
			})
		}

		// 3.Entropy based detection
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
func ScanFile(ctx context.Context, filePath string, commit string, message string) []types.Finding {
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	s := bufio.NewScanner(file)
	return scanLines(ctx, s, filePath, commit, message)
}

// ScanContent scans in-memory content (used for git history trees where files are not checked out).
func ScanContent(ctx context.Context, content string, virtualPath string, commit string, message string) []types.Finding {
	s := bufio.NewScanner(strings.NewReader(content))
	return scanLines(ctx, s, virtualPath, commit, message)
}
