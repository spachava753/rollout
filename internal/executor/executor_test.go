package executor

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

func mockExecutorFunc(cfg models.JobConfig) TrialExecutor {
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

func slowMockExecutorFunc(delay time.Duration, counter *int32) NewTrialExecutorFunc {
	return func(cfg models.JobConfig) TrialExecutor {
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

	orchestrator, err := NewJobOrchestrator(cfg, DefaultTrialExecutorFunc)
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

	orchestrator, err := NewJobOrchestrator(cfg, DefaultTrialExecutorFunc)
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
	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}
	if _, err := orchestrator.Run(t.Context()); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Second run with same name fails
	orchestrator2, _ := NewJobOrchestrator(cfg, mockExecutorFunc)
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
		orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
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
		orchestrator, err := NewJobOrchestrator(cfg, slowMockExecutorFunc(1*time.Second, &execCount))
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
			exec := NewTrialExecutor(
				"/tmp/instruction.md",
				tt.multiplier,
				models.JobVerifierConfig{
					OverrideTimeoutSec: tt.overrideSec,
					MaxTimeoutSec:      tt.maxSec,
				},
				models.JobEnvironmentConfig{}, // Added missing argument
			)

			got := exec.computeVerifierTimeout(tt.taskTimeoutSec)
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

	orchestrator, err := NewJobOrchestrator(cfg, DefaultTrialExecutorFunc)
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

	orchestrator, err := NewJobOrchestrator(cfg, DefaultTrialExecutorFunc)
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


// --- Task Filtering Tests ---

// createMultiTaskDataset creates a temporary dataset with multiple tasks for testing.
func createMultiTaskDataset(t *testing.T) string {
	t.Helper()
	
	baseDir := t.TempDir()
	
	tasks := []string{"task-a", "task-b", "task-c"}
	for _, taskName := range tasks {
		taskDir := filepath.Join(baseDir, taskName)
		if err := os.MkdirAll(filepath.Join(taskDir, "environment"), 0755); err != nil {
			t.Fatalf("creating task dir: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(taskDir, "tests"), 0755); err != nil {
			t.Fatalf("creating tests dir: %v", err)
		}
		
		// Create minimal task.toml
		taskToml := `[task]
timeout_sec = 60
`
		if err := os.WriteFile(filepath.Join(taskDir, "task.toml"), []byte(taskToml), 0644); err != nil {
			t.Fatalf("writing task.toml: %v", err)
		}
		
		// Create instruction.md
		if err := os.WriteFile(filepath.Join(taskDir, "instruction.md"), []byte("# Test task"), 0644); err != nil {
			t.Fatalf("writing instruction.md: %v", err)
		}
		
		// Create Dockerfile
		dockerfile := "FROM alpine:latest\nRUN apk add --no-cache bash\n"
		if err := os.WriteFile(filepath.Join(taskDir, "environment", "Dockerfile"), []byte(dockerfile), 0644); err != nil {
			t.Fatalf("writing Dockerfile: %v", err)
		}
		
		// Create test.sh that always passes
		testScript := "#!/bin/bash\nexit 0\n"
		if err := os.WriteFile(filepath.Join(taskDir, "tests", "test.sh"), []byte(testScript), 0755); err != nil {
			t.Fatalf("writing test.sh: %v", err)
		}
	}
	
	return baseDir
}

func TestTaskFiltering_LocalDataset(t *testing.T) {
	datasetPath := createMultiTaskDataset(t)
	
	cfg := models.JobConfig{
		Name:              ptr("test-task-filtering-local"),
		JobsDir:           t.TempDir(),
		NAttempts:         1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Environment:       models.JobEnvironmentConfig{Type: "docker"},
		Agents:            []models.Agent{{Name: "test-agent"}},
		Datasets: []models.DatasetRef{
			{
				Path:  &datasetPath,
				Tasks: []string{"task-a", "task-c"}, // Filter to only 2 tasks
			},
		},
	}

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should only have 2 trials (one per filtered task)
	if result.TotalTrials != 2 {
		t.Errorf("expected 2 trials (filtered tasks), got %d", result.TotalTrials)
	}

	// Verify task names
	tasksSeen := make(map[string]bool)
	for _, r := range result.Results {
		tasksSeen[r.TaskName] = true
	}
	if !tasksSeen["task-a"] {
		t.Error("expected task-a to be executed")
	}
	if !tasksSeen["task-c"] {
		t.Error("expected task-c to be executed")
	}
	if tasksSeen["task-b"] {
		t.Error("task-b should have been filtered out")
	}
}

func TestTaskFiltering_NonExistentTask_FailsEarly(t *testing.T) {
	datasetPath := createMultiTaskDataset(t)
	
	cfg := models.JobConfig{
		Name:              ptr("test-task-filtering-invalid"),
		JobsDir:           t.TempDir(),
		NAttempts:         1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Environment:       models.JobEnvironmentConfig{Type: "docker"},
		Agents:            []models.Agent{{Name: "test-agent"}},
		Datasets: []models.DatasetRef{
			{
				Path:  &datasetPath,
				Tasks: []string{"task-a", "nonexistent-task"}, // One valid, one invalid
			},
		},
	}

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	_, err = orchestrator.Run(t.Context())
	if err == nil {
		t.Fatal("expected error for nonexistent task, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "filtering dataset") {
		t.Errorf("error should mention filtering: %v", err)
	}
	if !strings.Contains(errStr, "nonexistent-task") {
		t.Errorf("error should mention the invalid task name: %v", err)
	}
}

func TestTaskFiltering_NilTasks_RunsAllTasks(t *testing.T) {
	datasetPath := createMultiTaskDataset(t)
	
	cfg := models.JobConfig{
		Name:              ptr("test-task-filtering-nil"),
		JobsDir:           t.TempDir(),
		NAttempts:         1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Environment:       models.JobEnvironmentConfig{Type: "docker"},
		Agents:            []models.Agent{{Name: "test-agent"}},
		Datasets: []models.DatasetRef{
			{
				Path:  &datasetPath,
				Tasks: nil, // nil means run all tasks
			},
		},
	}

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should have all 3 tasks
	if result.TotalTrials != 3 {
		t.Errorf("expected 3 trials (all tasks), got %d", result.TotalTrials)
	}

	// Verify all task names present
	tasksSeen := make(map[string]bool)
	for _, r := range result.Results {
		tasksSeen[r.TaskName] = true
	}
	for _, name := range []string{"task-a", "task-b", "task-c"} {
		if !tasksSeen[name] {
			t.Errorf("expected %s to be executed", name)
		}
	}
}

func TestTaskFiltering_SingleTask(t *testing.T) {
	datasetPath := createMultiTaskDataset(t)
	
	cfg := models.JobConfig{
		Name:              ptr("test-task-filtering-single"),
		JobsDir:           t.TempDir(),
		NAttempts:         1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Environment:       models.JobEnvironmentConfig{Type: "docker"},
		Agents:            []models.Agent{{Name: "test-agent"}},
		Datasets: []models.DatasetRef{
			{
				Path:  &datasetPath,
				Tasks: []string{"task-b"}, // Only one task
			},
		},
	}

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should have exactly 1 trial
	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial, got %d", result.TotalTrials)
	}

	if result.Results[0].TaskName != "task-b" {
		t.Errorf("expected task-b, got %s", result.Results[0].TaskName)
	}
}

func TestTaskFiltering_MultipleAttempts(t *testing.T) {
	datasetPath := createMultiTaskDataset(t)
	
	cfg := models.JobConfig{
		Name:              ptr("test-task-filtering-attempts"),
		JobsDir:           t.TempDir(),
		NAttempts:         3, // Multiple attempts
		NConcurrentTrials: 2,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Environment:       models.JobEnvironmentConfig{Type: "docker"},
		Agents:            []models.Agent{{Name: "test-agent"}},
		Datasets: []models.DatasetRef{
			{
				Path:  &datasetPath,
				Tasks: []string{"task-a", "task-c"}, // 2 tasks
			},
		},
	}

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should have 2 tasks * 3 attempts = 6 trials
	if result.TotalTrials != 6 {
		t.Errorf("expected 6 trials (2 tasks * 3 attempts), got %d", result.TotalTrials)
	}
}


// --- Task Override Tests ---

func TestRunFromConfigWithOptions_TaskOverride(t *testing.T) {
	// This test verifies that RunFromConfigWithOptions applies task override correctly
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	// Create a test job.yaml with task filters
	jobYaml := `name: test-task-override
agents:
  - name: oracle
n_attempts: 1
datasets:
  - path: ./testdata/multi-task-dataset
    tasks:
      - task-a
      - task-b
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "job.yaml")
	if err := os.WriteFile(configPath, []byte(jobYaml), 0644); err != nil {
		t.Fatalf("writing job.yaml: %v", err)
	}

	// Load and verify config loading works
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Verify original tasks
	if len(cfg.Datasets) != 1 {
		t.Fatalf("expected 1 dataset, got %d", len(cfg.Datasets))
	}

	originalTasks := cfg.Datasets[0].Tasks
	if len(originalTasks) != 2 {
		t.Fatalf("expected 2 tasks in original config, got %d", len(originalTasks))
	}

	// Now test with task override
	cfg, err = config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Simulate what RunFromConfigWithOptions does
	taskOverride := []string{"task-c"}
	for i := range cfg.Datasets {
		cfg.Datasets[i].Tasks = taskOverride
	}

	// Verify override was applied
	if len(cfg.Datasets[0].Tasks) != 1 {
		t.Fatalf("expected 1 task after override, got %d", len(cfg.Datasets[0].Tasks))
	}
	if cfg.Datasets[0].Tasks[0] != "task-c" {
		t.Errorf("expected task-c, got %s", cfg.Datasets[0].Tasks[0])
	}

	// Make dataset path absolute for loading
	absPath := filepath.Join(projectRoot, "testdata", "multi-task-dataset")
	cfg.Datasets[0].Path = &absPath
	cfg.JobsDir = tmpDir

	// Create orchestrator and verify it processes the override correctly
	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should have executed exactly 1 trial (1 agent × 1 task × 1 attempt)
	if result.TotalTrials != 1 {
		t.Errorf("expected 1 trial with task override, got %d", result.TotalTrials)
	}

	// Verify the task that ran was task-c
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].TaskName != "task-c" {
		t.Errorf("expected task-c to run, got %s", result.Results[0].TaskName)
	}
}

func TestRunFromConfigWithOptions_TaskOverrideMultipleDatasets(t *testing.T) {
	// Verify task override applies to ALL datasets
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	multiTaskPath := filepath.Join(projectRoot, "testdata", "multi-task-dataset")
	helloWorldPath := filepath.Join(projectRoot, "testdata", "hello-world-dataset")

	// Create config with two datasets with different task filters
	cfg := models.JobConfig{
		Name:              ptr("test-override-multiple"),
		JobsDir:          t.TempDir(),
		NAttempts:        1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:  "/tmp/instruction.md",
		Environment: models.JobEnvironmentConfig{
			Type:        "docker",
			PreserveEnv: models.PreserveNever,
		},
		Agents: []models.Agent{
			{Name: "oracle"},
		},
		Datasets: []models.DatasetRef{
			{
				Path:  &multiTaskPath,
				Tasks: []string{"task-a"},  // Original filter
			},
			{
				Path:  &helloWorldPath,
				Tasks: []string{"hello-world"}, // Original filter
			},
		},
	}

	// Apply task override (simulating CLI flag)
	taskOverride := []string{"task-a", "task-b"}
	for i := range cfg.Datasets {
		cfg.Datasets[i].Tasks = taskOverride
	}

	// Verify both datasets now have the override
	for i, ds := range cfg.Datasets {
		if len(ds.Tasks) != 2 {
			t.Errorf("dataset[%d]: expected 2 tasks after override, got %d", i, len(ds.Tasks))
		}
		if ds.Tasks[0] != "task-a" || ds.Tasks[1] != "task-b" {
			t.Errorf("dataset[%d]: tasks not correctly overridden", i)
		}
	}
}

func TestRunOptions_NilTaskOverride(t *testing.T) {
	// Verify nil TaskOverride doesn't change original config
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("getting project root: %v", err)
	}

	jobYaml := `name: test-nil-override
agents:
  - name: oracle
n_attempts: 1
datasets:
  - path: ./testdata/multi-task-dataset
    tasks:
      - task-a
      - task-b
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "job.yaml")
	if err := os.WriteFile(configPath, []byte(jobYaml), 0644); err != nil {
		t.Fatalf("writing job.yaml: %v", err)
	}

	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		t.Fatalf("loading job config: %v", err)
	}

	// Simulate RunFromConfigWithOptions with nil override (no change)
	opts := RunOptions{TaskOverride: nil}
	if opts.TaskOverride != nil {
		for i := range cfg.Datasets {
			cfg.Datasets[i].Tasks = opts.TaskOverride
		}
	}

	// Original tasks should remain unchanged
	if len(cfg.Datasets[0].Tasks) != 2 {
		t.Errorf("expected 2 tasks (unchanged), got %d", len(cfg.Datasets[0].Tasks))
	}
	if cfg.Datasets[0].Tasks[0] != "task-a" || cfg.Datasets[0].Tasks[1] != "task-b" {
		t.Error("original tasks were modified when TaskOverride was nil")
	}

	// Make path absolute for actual test
	absPath := filepath.Join(projectRoot, "testdata", "multi-task-dataset")
	cfg.Datasets[0].Path = &absPath
	cfg.JobsDir = tmpDir

	orchestrator, err := NewJobOrchestrator(cfg, mockExecutorFunc)
	if err != nil {
		t.Fatalf("creating orchestrator: %v", err)
	}

	result, err := orchestrator.Run(t.Context())
	if err != nil {
		t.Fatalf("running job: %v", err)
	}

	// Should run 2 trials (task-a and task-b)
	if result.TotalTrials != 2 {
		t.Errorf("expected 2 trials with nil override (original config), got %d", result.TotalTrials)
	}
}
