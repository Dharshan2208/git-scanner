package detector

import (
	"encoding/json"
	"log"
	"os"
	"regexp"
)

type rawSignature struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

type Signature struct {
	Name  string
	Regex *regexp.Regexp
}

var Signatures []Signature

// Load signatures from JSON
func LoadSignatures(path string) {
	file, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var data struct {
		Signatures []rawSignature `json:"signatures"`
	}

	if err := json.Unmarshal(file, &data); err != nil {
		log.Fatal(err)
	}

	for _, sig := range data.Signatures {
		re := regexp.MustCompile(sig.Pattern)

		Signatures = append(Signatures, Signature{
			Name:  sig.Name,
			Regex: re,
		})
	}

	log.Println("Loaded signatures:", len(Signatures))
}
