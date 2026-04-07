package detector

import (
	"math"
	"regexp"
)

// Match long tokens (possible secrets)
var tokenRegex = regexp.MustCompile(`[A-Za-z0-9+/=_-]{20,}`)

// Shannon entropy calculation
func calculateEntropy(s string) float64 {
	freq := make(map[rune]float64)

	for _, c := range s {
		freq[c]++
	}

	var entropy float64
	length := float64(len(s))

	for _, count := range freq {
		p := count / length
		entropy -= p * math.Log2(p)
	}

	return entropy
}

// FindHighEntropy detects suspicious random strings
func FindHighEntropy(line string) []string {
	var results []string

	tokens := tokenRegex.FindAllString(line, -1)

	for _, token := range tokens {
		if len(token) < 20 {
			continue
		}

		if calculateEntropy(token) > 4.5 {
			results = append(results, token)
		}
	}

	return results
}