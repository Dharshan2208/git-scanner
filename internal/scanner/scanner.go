package scanner

import (
	"bufio"
	"os"

	"github.com/Dharshan2208/git-scanner/internal/detector"
	"github.com/Dharshan2208/git-scanner/internal/types"
)

// scans a file and returns findings
func ScanFile(filePath string) []types.Finding {
	var findings []types.Finding

	file, err := os.Open(filePath)
	if err != nil {
		return findings
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		line := scanner.Text()

		for _, sig := range detector.Signatures {
			if sig.Regex.MatchString(line) {
				findings = append(findings, types.Finding{
					File:  filePath,
					Line:  lineNum,
					Type:  sig.Name,
					Match: line,
				})
			}
		}

		lineNum++
	}

	return findings
}
