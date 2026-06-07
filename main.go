package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Dharshan2208/git-scanner/cmd"
	"github.com/Dharshan2208/git-scanner/internal/detector"
)

func main() {
	detector.LoadSignatures()

	// Create a context that is cancelled on SIGINT or SIGTERM.
	// This allows graceful shutdown of in-flight scans.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Execute the CLI with the cancellable context.
	// Cobra's ExecuteContext will propagate the context to commands.
	if err := cmd.ExecuteContext(ctx); err != nil {
		// If the context was cancelled, the user likely pressed Ctrl+C.
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "\nScan cancelled by user.")
			os.Exit(130) // 128 + SIGINT(2)
		}
		log.Fatal(err)
	}
}
