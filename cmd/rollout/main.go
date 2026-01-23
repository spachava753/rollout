package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/executor"
)

func main() {
	// Define flags
	tasksFlag := flag.String("tasks", "", "Override task selection (comma-separated task names)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rollout [options] <job.yaml>")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Validate positional argument
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	configPath := flag.Arg(0)

	// Parse and validate --tasks flag
	var taskOverride []string
	if *tasksFlag != "" {
		taskOverride = strings.Split(*tasksFlag, ",")
		// Trim whitespace from each task name
		for i := range taskOverride {
			taskOverride[i] = strings.TrimSpace(taskOverride[i])
		}
		// Validate task names
		if err := config.ValidateTaskNames(taskOverride); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

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

	opts := executor.RunOptions{
		TaskOverride: taskOverride,
	}

	result, err := executor.RunFromConfigWithOptions(ctx, configPath, opts)
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
