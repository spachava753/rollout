package executor_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/environment"
	"github.com/spachava753/rollout/internal/executor"
	"github.com/spachava753/rollout/internal/models"
)

var testJobsDir = flag.String("test.jobsdir", "", "directory to preserve test job outputs (default: temp dir)")

// getJobsDir returns the jobs directory for tests.
// If -test.jobsdir flag is set, uses that directory, otherwise creates a temp dir.
func getJobsDir(t *testing.T) string {
	if *testJobsDir != "" {
		// Use the provided directory
		absPath, err := filepath.Abs(*testJobsDir)
		if err != nil {
			t.Fatalf("getting absolute path for jobs dir: %v", err)
		}

		// Create if doesn't exist
		if err := os.MkdirAll(absPath, 0755); err != nil {
			t.Fatalf("creating jobs dir: %v", err)
		}

		return absPath
	}

	// Use temp directory
	return t.TempDir()
}

func TestOracleAgentHelloWorld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Get project root
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	configPath := filepath.Join(projectRoot, "testdata", "job.yaml")
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Set jobs dir to temp dir or flag-specified dir
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-oracle-hello-world")

	// Ensure dataset path is absolute
	for i, ds := range cfg.Datasets {
		if ds.Path != nil && !filepath.IsAbs(*ds.Path) {
			absPath := filepath.Join(projectRoot, *ds.Path)
			cfg.Datasets[i].Path = &absPath
		}
	}

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(ctx)
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Verify results
	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial, got %d", result.TotalTrials)
	}

	if result.CompletedTrials != 1 {
		t.Errorf("expected 1 completed trial, got %d", result.CompletedTrials)
	}

	if result.FailedTrials != 0 {
		t.Errorf("expected 0 failed trials, got %d", result.FailedTrials)
	}

	if result.PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f%%", result.PassRate*100)
	}

	if result.MeanReward != 1.0 {
		t.Errorf("expected mean reward 1.0, got %f", result.MeanReward)
	}

	// Check agent summary
	oracleSummary, ok := result.Agents["oracle"]
	if !ok {
		t.Error("oracle agent summary not found")
	} else {
		if oracleSummary.TotalTrials != 1 {
			t.Errorf("oracle: expected 1 trial, got %d", oracleSummary.TotalTrials)
		}
		if oracleSummary.PassRate != 1.0 {
			t.Errorf("oracle: expected 100%% pass rate, got %.2f%%", oracleSummary.PassRate*100)
		}
	}

	t.Logf("Job completed successfully:")
	t.Logf("  Total trials: %d", result.TotalTrials)
	t.Logf("  Completed: %d", result.CompletedTrials)
	t.Logf("  Failed: %d", result.FailedTrials)
	t.Logf("  Pass rate: %.2f%%", result.PassRate*100)
	t.Logf("  Mean reward: %f", result.MeanReward)
}

func TestOracleAgentMultipleAttempts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Get project root
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	configPath := filepath.Join(projectRoot, "testdata", "job.yaml")
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Configure multiple attempts
	cfg.NAttempts = 3

	// Set jobs dir to temp dir or flag-specified dir
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-multiple-attempts")

	// Ensure dataset path is absolute
	for i, ds := range cfg.Datasets {
		if ds.Path != nil && !filepath.IsAbs(*ds.Path) {
			absPath := filepath.Join(projectRoot, *ds.Path)
			cfg.Datasets[i].Path = &absPath
		}
	}

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(ctx)
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Verify total trials equals tasks * attempts
	expectedTrials := cfg.NAttempts // 1 task × 3 attempts
	if result.TotalTrials != expectedTrials {
		t.Errorf("expected %d trials, got %d", expectedTrials, result.TotalTrials)
	}

	if result.CompletedTrials != expectedTrials {
		t.Errorf("expected %d completed trials, got %d", expectedTrials, result.CompletedTrials)
	}

	if result.FailedTrials != 0 {
		t.Errorf("expected 0 failed trials, got %d", result.FailedTrials)
	}

	if result.PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f%%", result.PassRate*100)
	}

	if result.MeanReward != 1.0 {
		t.Errorf("expected mean reward 1.0, got %f", result.MeanReward)
	}

	// Verify each attempt is recorded in results
	attemptsSeen := make(map[int]bool)
	for _, r := range result.Results {
		if r.AgentName != "oracle" {
			t.Errorf("expected agent 'oracle', got '%s'", r.AgentName)
		}
		if r.Reward == nil {
			t.Errorf("attempt %d: expected reward, got nil", r.Attempt)
		} else if *r.Reward != 1.0 {
			t.Errorf("attempt %d: expected reward 1.0, got %f", r.Attempt, *r.Reward)
		}
		attemptsSeen[r.Attempt] = true
	}

	// Verify all attempts (1, 2, 3) were executed
	for i := 1; i <= cfg.NAttempts; i++ {
		if !attemptsSeen[i] {
			t.Errorf("attempt %d not found in results", i)
		}
	}

	// Check agent summary reflects all attempts
	oracleSummary, ok := result.Agents["oracle"]
	if !ok {
		t.Fatal("oracle agent summary not found")
	}

	if oracleSummary.TotalTrials != expectedTrials {
		t.Errorf("oracle: expected %d trials, got %d", expectedTrials, oracleSummary.TotalTrials)
	}
	if oracleSummary.CompletedTrials != expectedTrials {
		t.Errorf("oracle: expected %d completed trials, got %d", expectedTrials, oracleSummary.CompletedTrials)
	}
	if oracleSummary.PassRate != 1.0 {
		t.Errorf("oracle: expected 100%% pass rate, got %.2f%%", oracleSummary.PassRate*100)
	}
	if oracleSummary.MeanReward != 1.0 {
		t.Errorf("oracle: expected mean reward 1.0, got %f", oracleSummary.MeanReward)
	}

	t.Logf("Multiple attempts test completed successfully:")
	t.Logf("  Total trials: %d", result.TotalTrials)
	t.Logf("  Completed: %d", result.CompletedTrials)
	t.Logf("  Pass rate: %.2f%%", result.PassRate*100)
	t.Logf("  Mean reward: %f", result.MeanReward)
}

// mockTrialExecutor is a test executor that does nothing.
type mockTrialExecutor struct{}

func (m *mockTrialExecutor) Execute(ctx context.Context, trial models.Trial, provider environment.Provider) (*models.TrialResult, error) {
	reward := 1.0
	return &models.TrialResult{
		TaskName:    trial.Task.Name,
		DatasetName: trial.Dataset,
		AgentName:   trial.Agent.Name,
		Attempt:     trial.Attempt,
		Reward:      &reward,
	}, nil
}

func mockExecutorFunc(cfg models.JobConfig) executor.TrialExecutor {
	return &mockTrialExecutor{}
}

func TestJobDirectoryOverwriteProtection(t *testing.T) {
	ctx := context.Background()

	// Get project root
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	configPath := filepath.Join(projectRoot, "testdata", "job.yaml")
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Set jobs dir and fixed job name
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-overwrite-protection")

	// Ensure dataset path is absolute
	for i, ds := range cfg.Datasets {
		if ds.Path != nil && !filepath.IsAbs(*ds.Path) {
			absPath := filepath.Join(projectRoot, *ds.Path)
			cfg.Datasets[i].Path = &absPath
		}
	}

	// First run - should succeed (using mock executor to avoid expensive operations)
	orchestrator, err := executor.NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(ctx)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial, got %d", result.TotalTrials)
	}

	// Second run with same job name - should fail
	orchestrator2, err := executor.NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating second orchestrator: %v", err)
	}

	result2, err := orchestrator2.Run(ctx)
	if err == nil {
		t.Fatal("expected error on second run, but got none")
	}

	if result2 != nil {
		t.Error("expected nil result on error, but got result")
	}

	// Verify error message mentions directory exists
	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") {
		t.Errorf("expected error about directory already existing, got: %s", errMsg)
	}

	t.Logf("Protection working correctly - prevented overwrite with error: %v", err)
}

func ptr[T any](v T) *T {
	return &v
}
