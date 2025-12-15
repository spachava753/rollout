package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spachava753/rollout/internal/executor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rollout <job.yaml>")
		os.Exit(1)
	}

	configPath := os.Args[1]

	// Setup context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	result, err := executor.RunFromConfig(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Printf("\nJob: %s\n", result.JobName)
	fmt.Printf("Total trials: %d\n", result.TotalTrials)
	fmt.Printf("Completed: %d\n", result.CompletedTrials)
	fmt.Printf("Failed: %d\n", result.FailedTrials)
	fmt.Printf("Pass rate: %.2f%%\n", result.PassRate*100)
	fmt.Printf("Mean reward: %.4f\n", result.MeanReward)
	fmt.Printf("Duration: %.2fs\n", result.TotalDurationSec)

	if result.FailedTrials > 0 || result.Cancelled {
		os.Exit(1)
	}
}
