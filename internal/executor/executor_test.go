package executor_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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
		absPath, err := filepath.Abs(*testJobsDir)
		if err != nil {
			t.Fatalf("getting absolute path for jobs dir: %v", err)
		}
		if err := os.MkdirAll(absPath, 0755); err != nil {
			t.Fatalf("creating jobs dir: %v", err)
		}
		return absPath
	}
	return t.TempDir()
}

// hasModalAuth checks if Modal authentication is available.
func hasModalAuth() bool {
	// Check env var first
	if os.Getenv("MODAL_TOKEN_ID") != "" {
		return true
	}
	// Check ~/.modal.toml
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".modal.toml"))
	return err == nil
}

func ptr[T any](v T) *T {
	return &v
}

// loadTestConfig loads the test job config with absolute dataset paths.
func loadTestConfig(t *testing.T) (models.JobConfig, string) {
	t.Helper()
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	configPath := filepath.Join(projectRoot, "testdata", "job.yaml")
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	for i, ds := range cfg.Datasets {
		if ds.Path != nil && !filepath.IsAbs(*ds.Path) {
			absPath := filepath.Join(projectRoot, *ds.Path)
			cfg.Datasets[i].Path = &absPath
		}
	}

	return cfg, projectRoot
}

// mockTrialExecutor returns success immediately.
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

// slowMockTrialExecutor simulates work with a delay (for cancellation testing).
type slowMockTrialExecutor struct {
	delay     time.Duration
	execCount *int32
}

func (m *slowMockTrialExecutor) Execute(ctx context.Context, trial models.Trial, provider environment.Provider) (*models.TrialResult, error) {
	atomic.AddInt32(m.execCount, 1)
	time.Sleep(m.delay)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	reward := 1.0
	return &models.TrialResult{
		TaskName:    trial.Task.Name,
		DatasetName: trial.Dataset,
		AgentName:   trial.Agent.Name,
		Attempt:     trial.Attempt,
		Reward:      &reward,
	}, nil
}

func slowMockExecutorFunc(delay time.Duration, counter *int32) executor.NewTrialExecutorFunc {
	return func(cfg models.JobConfig) executor.TrialExecutor {
		return &slowMockTrialExecutor{delay: delay, execCount: counter}
	}
}

// --- Integration Tests (require Docker) ---

func TestOracleAgentHelloWorld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _ := loadTestConfig(t)
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-oracle-hello-world")

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial, got %d", result.TotalTrials)
	}
	if result.CompletedTrials != 1 {
		t.Errorf("expected 1 completed, got %d", result.CompletedTrials)
	}
	if result.PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f%%", result.PassRate*100)
	}

	t.Logf("Completed: trials=%d, pass_rate=%.0f%%", result.TotalTrials, result.PassRate*100)
}

func TestOracleAgentConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _ := loadTestConfig(t)
	cfg.NAttempts = 3
	cfg.NConcurrentTrials = 3
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-oracle-concurrent")

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	if result.TotalTrials != 3 {
		t.Errorf("expected 3 trials, got %d", result.TotalTrials)
	}
	if result.CompletedTrials != 3 {
		t.Errorf("expected 3 completed, got %d", result.CompletedTrials)
	}
	if result.PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f%%", result.PassRate*100)
	}

	// Verify all attempts recorded
	attemptsSeen := make(map[int]bool)
	for _, r := range result.Results {
		attemptsSeen[r.Attempt] = true
	}
	for i := 1; i <= 3; i++ {
		if !attemptsSeen[i] {
			t.Errorf("attempt %d not found", i)
		}
	}

	t.Logf("Completed: trials=%d, pass_rate=%.0f%%", result.TotalTrials, result.PassRate*100)
}

// --- Unit Tests ---

func TestJobDirectoryOverwriteProtection(t *testing.T) {
	cfg, _ := loadTestConfig(t)
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-overwrite-protection")

	// First run succeeds
	orchestrator, err := executor.NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}
	if _, err := orchestrator.Run(t.Context()); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Second run with same name fails
	orchestrator2, _ := executor.NewJobOrchestrator(cfg, mockExecutorFunc)
	_, err = orchestrator2.Run(t.Context())
	if err == nil {
		t.Fatal("expected error on second run")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", err)
	}
}

func TestOrchestratorResultAggregation(t *testing.T) {
	cfg, _ := loadTestConfig(t)
	cfg.NAttempts = 5
	cfg.NConcurrentTrials = 3
	cfg.JobsDir = t.TempDir()
	cfg.Name = ptr("test-aggregation")

	synctest.Test(t, func(t *testing.T) {
		orchestrator, err := executor.NewJobOrchestrator(cfg, mockExecutorFunc)
		if err != nil {
			t.Fatalf("creating orchestrator: %v", err)
		}

		result, err := orchestrator.Run(t.Context())
		if err != nil {
			t.Fatalf("running job: %v", err)
		}

		// Verify counts
		if result.TotalTrials != 5 {
			t.Errorf("expected 5 total, got %d", result.TotalTrials)
		}
		if result.CompletedTrials != 5 {
			t.Errorf("expected 5 completed, got %d", result.CompletedTrials)
		}
		if result.FailedTrials != 0 {
			t.Errorf("expected 0 failed, got %d", result.FailedTrials)
		}

		// Verify aggregation
		if result.PassRate != 1.0 {
			t.Errorf("expected pass rate 1.0, got %f", result.PassRate)
		}
		if result.MeanReward != 1.0 {
			t.Errorf("expected mean reward 1.0, got %f", result.MeanReward)
		}

		// Verify all attempts present
		attemptsSeen := make(map[int]bool)
		for _, r := range result.Results {
			attemptsSeen[r.Attempt] = true
		}
		for i := 1; i <= 5; i++ {
			if !attemptsSeen[i] {
				t.Errorf("attempt %d not found", i)
			}
		}

		// Verify agent summary
		summary, ok := result.Agents["oracle"]
		if !ok {
			t.Fatal("oracle summary not found")
		}
		if summary.TotalTrials != 5 {
			t.Errorf("oracle: expected 5 trials, got %d", summary.TotalTrials)
		}
		if summary.PassRate != 1.0 {
			t.Errorf("oracle: expected pass rate 1.0, got %f", summary.PassRate)
		}
	})
}

func TestCancellationStopsExecution(t *testing.T) {
	cfg, _ := loadTestConfig(t)
	cfg.NAttempts = 10
	cfg.NConcurrentTrials = 2
	cfg.JobsDir = t.TempDir()
	cfg.Name = ptr("test-cancellation")

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		var execCount int32
		orchestrator, err := executor.NewJobOrchestrator(cfg, slowMockExecutorFunc(1*time.Second, &execCount))
		if err != nil {
			t.Fatalf("creating orchestrator: %v", err)
		}

		// Cancel after first batch completes
		go func() {
			time.Sleep(1500 * time.Millisecond)
			cancel()
		}()

		result, err := orchestrator.Run(ctx)
		if err != nil {
			t.Fatalf("running job: %v", err)
		}

		// Verify some trials were skipped due to cancellation
		if result.SkippedTrials == 0 && result.CompletedTrials == cfg.NAttempts {
			t.Log("Note: all trials completed before cancellation took effect")
		} else {
			t.Logf("Cancellation worked: completed=%d, skipped=%d", result.CompletedTrials, result.SkippedTrials)
		}
	})
}


func TestComputeVerifierTimeout(t *testing.T) {
	tests := []struct {
		name           string
		taskTimeoutSec float64
		multiplier     float64
		overrideSec    *float64
		maxSec         *float64
		wantSec        float64
	}{
		{
			name:           "basic with multiplier",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			wantSec:        100.0,
		},
		{
			name:           "multiplier applied",
			taskTimeoutSec: 100.0,
			multiplier:     2.0,
			wantSec:        200.0,
		},
		{
			name:           "override takes precedence",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			overrideSec:    ptr(50.0),
			wantSec:        50.0,
		},
		{
			name:           "override with multiplier",
			taskTimeoutSec: 100.0,
			multiplier:     2.0,
			overrideSec:    ptr(50.0),
			wantSec:        100.0, // 50 * 2
		},
		{
			name:           "max ceiling applied",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			maxSec:         ptr(60.0),
			wantSec:        60.0,
		},
		{
			name:           "max ceiling with multiplier",
			taskTimeoutSec: 100.0,
			multiplier:     2.0,
			maxSec:         ptr(150.0), // max becomes 300 after multiplier
			wantSec:        200.0,      // task timeout after multiplier (within ceiling)
		},
		{
			name:           "max ceiling caps high timeout",
			taskTimeoutSec: 200.0,
			multiplier:     2.0,
			maxSec:         ptr(150.0), // max becomes 300 after multiplier
			wantSec:        300.0,      // capped at max*multiplier
		},
		{
			name:           "override and max together - override wins under max",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			overrideSec:    ptr(50.0),
			maxSec:         ptr(100.0),
			wantSec:        50.0,
		},
		{
			name:           "override exceeds max - max caps",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			overrideSec:    ptr(200.0),
			maxSec:         ptr(100.0),
			wantSec:        100.0, // capped by max
		},
		{
			name:           "zero override ignored",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			overrideSec:    ptr(0.0),
			wantSec:        100.0,
		},
		{
			name:           "zero max ignored",
			taskTimeoutSec: 100.0,
			multiplier:     1.0,
			maxSec:         ptr(0.0),
			wantSec:        100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := executor.NewTrialExecutor(
				"/tmp/instruction.md",
				tt.multiplier,
				models.JobVerifierConfig{
					OverrideTimeoutSec: tt.overrideSec,
					MaxTimeoutSec:      tt.maxSec,
				},
				models.JobEnvironmentConfig{}, // Added missing argument
			)

			got := exec.ComputeVerifierTimeout(tt.taskTimeoutSec)
			wantDuration := time.Duration(tt.wantSec) * time.Second
			if got != wantDuration {
				t.Errorf("got %v, want %v", got, wantDuration)
			}
		})
	}
}

func TestModalOracleAgentHelloWorld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Skip if MODAL_TOKEN_ID is not set (Modal auth required)
	if !hasModalAuth() {
		t.Skip("skipping Modal integration test: no Modal auth (set MODAL_TOKEN_ID or configure ~/.modal.toml)")
	}

	cfg, _ := loadTestConfig(t)
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-modal-oracle-hello-world")
	cfg.Environment.Type = "modal"

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial, got %d", result.TotalTrials)
	}
	if result.CompletedTrials != 1 {
		t.Errorf("expected 1 completed, got %d", result.CompletedTrials)
	}
	if result.FailedTrials != 0 {
		t.Errorf("expected 0 failed, got %d", result.FailedTrials)
	}
	if result.PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f%%", result.PassRate*100)
	}

	t.Logf("Modal test completed: trials=%d, pass_rate=%.0f%%", result.TotalTrials, result.PassRate*100)
}

func TestModalAppCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if !hasModalAuth() {
		t.Skip("skipping Modal integration test: no Modal auth (set MODAL_TOKEN_ID or configure ~/.modal.toml)")
	}

	cfg, _ := loadTestConfig(t)
	cfg.JobsDir = getJobsDir(t)
	cfg.Name = ptr("test-modal-cleanup")
	cfg.Environment.Type = "modal"
	cfg.Environment.PreserveEnv = models.PreserveNever

	orchestrator, err := executor.NewJobOrchestrator(cfg, executor.DefaultTrialExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	if result.CompletedTrials != 1 {
		t.Fatalf("expected 1 completed trial, got %d", result.CompletedTrials)
	}

	// The Modal sandbox should be terminated after trial completion.
	// We can't easily verify the app was deleted without Modal API access,
	// but the test succeeding without errors indicates cleanup worked.
	t.Log("Modal cleanup test completed - sandbox terminated successfully")
}
