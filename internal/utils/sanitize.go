package utils

import (
	"strings"
)

func SanitizeSecret(secret string) string {
	// Handle empty or very short strings
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}

	// Keep first 4 and last 4 characters visible
	keepVisible := 4
	if len(secret) < 10 {
		keepVisible = 2
	}

	if len(secret) <= keepVisible*2 {
		return strings.Repeat("*", len(secret))
	}

	// Detect if it's a multiline secret (like PEM certificates)
	if strings.Contains(secret, "\n") {
		return SanitizeMultilineSecret(secret)
	}

	// Detect common prefixes for better display
	prefix := detectPrefix(secret)
	prefixLen := len(prefix)

	// Skip prefix when taking visible characters
	// But ensure we have enough content after the prefix
	remainingAfterPrefix := len(secret) - prefixLen
	minContent := keepVisible*2 + 3 // visible start + ... + visible end

	if remainingAfterPrefix < minContent {
		// Not enough content after prefix, just show prefix + asterisks
		return prefix + strings.Repeat("*", 4)
	}

	// Take visible characters AFTER the prefix
	visibleStart := secret[prefixLen : prefixLen+keepVisible]
	visibleEnd := secret[len(secret)-keepVisible:]

	return prefix + visibleStart + "..." + visibleEnd
}

// detectPrefix identifies common secret prefixes and returns them separately
func detectPrefix(secret string) string {
	commonPrefixes := []string{
		"sk-proj-",
		"sk-ant-api03-",
		"sk-",
		"ghp_",
		"gho_",
		"github_pat_",
		"glpat-",
		"xoxb-",
		"xoxp-",
		"xoxa-",
		"xoxr-",
		"xoxs-",
		"eyJ", // JWT prefix
		"AKIA",
		"ASIA",
		"dop_v1_",
		"pat",
		"whsec_",
		"sk_live_",
		"sk_test_",
		"rk_live_",
		"rk_test_",
		"hf_",
		"gsk_",
		"tvly-",
		"xai-",
		"AIza",
		"GOCSPX-",
		"ya29.",
		"PMAK-",
		"----",
	}

	for _, prefix := range commonPrefixes {
		if strings.HasPrefix(secret, prefix) {
			return prefix
		}
	}

	return ""
}

func SanitizeMultilineSecret(secret string) string {
	lines := strings.Split(secret, "\n")
	if len(lines) <= 2 {
		return strings.Repeat("*", 20) + " (multiline)"
	}

	// Keep first and last line header, redact content
	var result []string
	for i, line := range lines {
		if i == 0 {
			// Keep first line (header)
			if len(line) > 20 {
				result = append(result, line[:20]+"...")
			} else {
				result = append(result, line)
			}
		} else if i == len(lines)-1 {
			// Keep last line (footer)
			if len(line) > 20 {
				result = append(result, "..."+line[len(line)-20:])
			} else {
				result = append(result, line)
			}
		} else {
			// Redact middle lines
			result = append(result, "    *****")
		}
	}

	return strings.Join(result, "\n")
}

func SanitizeForJSON(text string) string {
	// Common patterns: "api_key": "value", api_key: "value", api_key=value
	patterns := []struct {
		prefix string
		suffix string
		minLen int
	}{
		{`"`, `"`, 8},
		{`": "`, `"`, 8},
		{"='", "'", 8},
		{"='", "'", 8},
		{"= \"", `"`, 8},
		{"=\"", `"`, 8},
		{": \"", `"`, 8},
		{": '", "'", 8},
	}

	for _, p := range patterns {
		text = sanitizePattern(text, p.prefix, p.suffix, p.minLen)
	}

	return text
}

func sanitizePattern(text, prefix, suffix string, minLen int) string {
	result := text
	for {
		idx := strings.Index(result, prefix)
		if idx == -1 {
			break
		}

		start := idx + len(prefix)
		// Find the end (suffix)
		end := start
		count := 0
		for end < len(result) {
			if result[end] == '\\' && end+1 < len(result) {
				end += 2 // Skip escaped characters
				continue
			}
			if strings.HasPrefix(result[end:], suffix) && count > minLen {
				break
			}
			end++
			count++
		}

		if end > start+minLen {
			original := result[start:end]
			sanitized := SanitizeSecret(original)
			result = result[:start] + sanitized + result[end:]
		} else {
			result = result[:idx+1] + strings.Repeat("*", 8) + result[idx+1:]
		}
	}
	return result
}

func ContainsOnlyAsterisks(s string) bool {
	for _, r := range s {
		if r != '*' && r != ' ' && r != '.' && r != '-' {
			return false
		}
	}
	return true
}
