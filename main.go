package main

import (
	"github.com/Dharshan2208/git-scanner/cmd"
	"github.com/Dharshan2208/git-scanner/internal/detector"
)

func main() {
	detector.LoadSignatures("internal/scanner/sign.json")
	cmd.Execute()
}
