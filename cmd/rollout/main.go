package main

import (
	"context"
	"fmt"
	"log/slog"
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

	// Setup context with manual signal handling
	ctx, cancel := context.WithCancel(context.Background())

	// Listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	defer func() {
		signal.Stop(sigChan)
		cancel()
	}()

	go func() {
		sig := <-sigChan
		slog.Info("interrupt received, shutting down gracefully...", "signal", sig)
		cancel()
	}()

	result, err := executor.RunFromConfig(ctx, configPath)
	if err != nil {
		slog.Error("job failed", "error", err)
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
