package detector

import (
	"regexp"
	"strings"
)

var multiWordKeywords = []string{
	"api_key", "apikey", "api-key",
	"secret_key", "secretkey", "secret-key",
	"passwd", "pwd",
	"auth_token", "access_token", "accesstoken",
	"client_secret", "clientsecret",
	"private_key", "privatekey",
	"aws_access_key_id", "aws_secret_access_key",
}

var genericKeywords = []string{
	"key", "secret", "password",
	"token", "auth", "authorization", "bearer",
	"credential", "credentials",
}

var multiWordRegexes []*regexp.Regexp
var genericKeywordRegexes []*regexp.Regexp

func init() {
	for _, kw := range multiWordKeywords {
		pattern := `\b` + regexp.QuoteMeta(kw) + `\b\s*[:=]\s*["']?([A-Za-z0-9+/=_\-.@:]{8,})["']?`
		multiWordRegexes = append(multiWordRegexes, regexp.MustCompile("(?i)"+pattern))
	}

	for _, kw := range genericKeywords {
		pattern := `\b` + regexp.QuoteMeta(kw) + `\b\s*[:=]\s*["']?([A-Za-z0-9+/=_\-.@:]{8,})["']?`
		genericKeywordRegexes = append(genericKeywordRegexes, regexp.MustCompile("(?i)"+pattern))
	}
}

// FindKeywordBased detects secrets using keyword patterns
// Uses two-tier approach:
// 1. Multi-word keywords (api_key, secret_key) - standard matching
// 2. Generic keywords (key, token) - strict matching to avoid false positives
func FindKeywordBased(line string) []string {
	var results []string

	for _, re := range multiWordRegexes {
		results = append(results, extractMatches(re, line, false, line)...)
	}

	for _, re := range genericKeywordRegexes {
		results = append(results, extractMatches(re, line, true, line)...)
	}

	return deduplicate(results)
}

func extractMatches(re *regexp.Regexp, line string, strictMode bool, originalLine string) []string {
	var matches []string
	for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
		if len(match) >= 4 {
			fullMatchStart := match[0]
			valueStart := match[2]
			valueEnd := match[3]

			if strictMode {
				if !isValidKeywordContext(originalLine, fullMatchStart) {
					continue
				}
			}

			value := line[valueStart:valueEnd]
			value = strings.Trim(value, `"' `)

			// Additional validation
			if isValidSecretValue(value) {
				matches = append(matches, value)
			}
		}
	}
	return matches
}

func isValidKeywordContext(line string, matchStart int) bool {
	if matchStart > 0 {
		prevChar := line[matchStart-1]
		if isAlphanumericOrUnderscore(prevChar) {
			return false
		}
	}
	return true
}

func isAlphanumericOrUnderscore(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}

func isValidSecretValue(value string) bool {
	// Minimum length check
	if len(value) < 8 {
		return false
	}

	upperValue := strings.ToUpper(value)
	if strings.Contains(upperValue, "XXXX") ||
		strings.Contains(upperValue, "CHANGEME") ||
		strings.Contains(upperValue, "YOUR_") ||
		strings.Contains(upperValue, "EXAMPLE") ||
		strings.Contains(upperValue, "TEST") ||
		strings.Contains(upperValue, "PLACEHOLDER") {
		return false
	}

	hasLetter := false
	hasNumber := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasNumber = true
		}
	}

	return hasLetter && hasNumber
}

func deduplicate(results []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, v := range results {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	return unique
}
