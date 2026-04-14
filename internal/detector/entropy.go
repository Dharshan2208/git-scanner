package detector

import (
	"math"
	"regexp"
	"strings"
)

// Match long tokens (possible secrets)
var tokenRegex = regexp.MustCompile(`[A-Za-z0-9+/=_-]{25,}`)

// calculateEntropy computes Shannon entropy of a string
func calculateEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}

	freq := make(map[rune]float64)
	for _, c := range s {
		freq[c]++
	}

	var entropy float64
	length := float64(len(s))

	for _, count := range freq {
		p := count / length
		// Skip zero probabilities to avoid log(0) which returns -Inf
		// and would corrupt the entropy calculation
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// FindHighEntropy detects suspicious high-entropy tokens with better filtering
func FindHighEntropy(line string) []string {
	var results []string

	tokens := tokenRegex.FindAllString(line, -1)

	for _, token := range tokens {
		if len(token) < 30 {
			continue
		}

		// Skip common false positives
		if isLikelyFalsePositive(token) {
			continue
		}

		entropy := calculateEntropy(token)

		// Higher threshold (5.0) + extra checks
		if entropy > 5.0 {
			results = append(results, token)
		}
	}

	return results
}

// isLikelyFalsePositive helps reduce noise from hashes, encoded data, etc.
func isLikelyFalsePositive(token string) bool {
	lower := strings.ToLower(token)

	// Skip common non-secret patterns
	if strings.Contains(lower, "sha256") ||
		strings.Contains(lower, "md5") ||
		strings.Contains(lower, "base64") ||
		strings.Contains(lower, "-----begin") ||
		strings.Contains(lower, "-----end") ||
		strings.HasPrefix(lower, "eyj") { // JWT header
		return true
	}

	// Too many repeated characters (low entropy in practice)
	if hasTooManyRepeats(token) {
		return true
	}

	return false
}

// hasTooManyRepeats skips strings with long runs of same character
func hasTooManyRepeats(s string) bool {
	count := 1
	maxRepeat := 1
	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			count++
			if count > maxRepeat {
				maxRepeat = count
			}
		} else {
			count = 1
		}
	}
	return maxRepeat > 6
}
