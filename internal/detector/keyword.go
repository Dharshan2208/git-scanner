package detector

import (
	"regexp"
	"strings"
)

var secretKeywords = []string{
	"api_key", "apikey", "api-key",
	"secret", "secret_key", "secretkey", "secret-key",
	"password", "passwd", "pwd",
	"token", "auth_token", "access_token", "accesstoken",
	"client_secret", "clientsecret",
	"private_key", "privatekey",
	"aws_access_key_id", "aws_secret_access_key",
	"key", "credential", "credentials",
	"auth", "authorization", "bearer",
}

var keywordRegexes []*regexp.Regexp

func init() {
	for _, kw := range secretKeywords {
		pattern := `(?i)\b` + regexp.QuoteMeta(kw) + `\b\s*[:=]\s*["']?([A-Za-z0-9+/=_-]{8,})["']?`
		keywordRegexes = append(keywordRegexes, regexp.MustCompile(pattern))
	}
}

func FindKeywordBased(line string) []string {
	var results []string

	for _, re := range keywordRegexes {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				value := strings.Trim(match[1], `"' `)

				// Basic length check + avoid false positives
				if len(value) >= 8 && !isLikelyFalsePositive(value) {
					results = append(results, value)
				}
			}
		}
	}

	return results
}
