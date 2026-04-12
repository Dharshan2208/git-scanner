package detector

import (
	"embed"
	"encoding/json"
	"log"
	"regexp"
)

// embedding so no need of sig.json
var signatureFS embed.FS

type rawSignature struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

type Signature struct {
	Name  string
	Regex *regexp.Regexp
}

var Signatures []Signature

func LoadSignatures() {
	// Reading the embedded file
	data, err := signatureFS.ReadFile("sign.json")
	if err != nil {
		log.Fatal("Failed to read embedded signatures:", err)
	}

	var config struct {
		Signatures []rawSignature `json:"signatures"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatal("Failed to parse signatures JSON:", err)
	}

	Signatures = Signatures[:0] // Clear existing signatures if reloading

	for _, sig := range config.Signatures {
		re, err := regexp.Compile(sig.Pattern)
		if err != nil {
			log.Printf("Warning: Invalid regex in signature '%s': %v", sig.Name, err)
			continue // Skip bad signature instead of crashing
		}

		Signatures = append(Signatures, Signature{
			Name:  sig.Name,
			Regex: re,
		})
	}

	log.Printf("Loaded %d signatures successfully", len(Signatures))
}
