package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/models"
)

func TestLoadTaskConfig(t *testing.T) {
	taskToml := `version = "1.0"

[metadata]
author_name = "Test Author"
difficulty = "easy"

[verifier]
timeout_sec = 120.0

[agent]
timeout_sec = 60.0
install_timeout_sec = 30.0

[environment]
cpus = 2
memory = "4G"
`

	fsys := fstest.MapFS{
		"task.toml": &fstest.MapFile{Data: []byte(taskToml)},
	}

	cfg, err := config.LoadTaskConfig(fsys)
	if err != nil {
		t.Fatalf("LoadTaskConfig failed: %v", err)
	}

	if cfg.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", cfg.Version)
	}

	if cfg.Verifier.TimeoutSec != 120.0 {
		t.Errorf("expected verifier timeout 120, got %f", cfg.Verifier.TimeoutSec)
	}

	if cfg.Agent.TimeoutSec != 60.0 {
		t.Errorf("expected agent timeout 60, got %f", cfg.Agent.TimeoutSec)
	}

	if cfg.Env.CPUs != 2 {
		t.Errorf("expected cpus 2, got %d", cfg.Env.CPUs)
	}

	// Should convert memory="4G" to memory_mb=4096
	if cfg.Env.MemoryMB != 4096 {
		t.Errorf("expected memory_mb 4096, got %d", cfg.Env.MemoryMB)
	}
}

func TestLoadTaskConfig_MemoryMB(t *testing.T) {
	taskToml := `version = "1.0"
[verifier]
timeout_sec = 600.0
[agent]
timeout_sec = 600.0
install_timeout_sec = 300.0
[environment]
cpus = 1
memory_mb = 4096
storage_mb = 8192
`

	fsys := fstest.MapFS{
		"task.toml": &fstest.MapFile{Data: []byte(taskToml)},
	}

	cfg, err := config.LoadTaskConfig(fsys)
	if err != nil {
		t.Fatalf("LoadTaskConfig failed: %v", err)
	}

    if cfg.Env.MemoryMB != 4096 {
        t.Errorf("expected memory_mb 4096, got %d", cfg.Env.MemoryMB)
    }
    
    if cfg.Env.StorageMB != 8192 {
        t.Errorf("expected storage_mb 8192, got %d", cfg.Env.StorageMB)
    }
}

func TestLoadJobConfig(t *testing.T) {
	jobYaml := `name: test-job
jobs_dir: test-output
n_attempts: 3
n_concurrent_trials: 4
timeout_multiplier: 1.5
instruction_path: /custom/instruction.md
environment:
  type: docker
  force_build: true
  preserve_env: on_failure
  override_cpus: 4
  override_memory_mb: 8192
  override_storage_mb: 16384
agents:
  - name: oracle
  - name: custom-agent
    install: "apt-get install -y curl"
    execute: "curl http://example.com"
    env:
      API_KEY: test-key
datasets:
  - path: ./test-dataset
`

	// Write to temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "job.yaml")
	if err := os.WriteFile(tmpFile, []byte(jobYaml), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	cfg, err := config.LoadJobConfig(tmpFile)
	if err != nil {
		t.Fatalf("LoadJobConfig failed: %v", err)
	}

	if *cfg.Name != "test-job" {
		t.Errorf("expected name test-job, got %s", *cfg.Name)
	}

	if cfg.JobsDir != "test-output" {
		t.Errorf("expected jobs_dir test-output, got %s", cfg.JobsDir)
	}

	if cfg.NAttempts != 3 {
		t.Errorf("expected n_attempts 3, got %d", cfg.NAttempts)
	}

	if cfg.NConcurrentTrials != 4 {
		t.Errorf("expected n_concurrent_trials 4, got %d", cfg.NConcurrentTrials)
	}

	if cfg.TimeoutMultiplier != 1.5 {
		t.Errorf("expected timeout_multiplier 1.5, got %f", cfg.TimeoutMultiplier)
	}

	if cfg.Environment.Type != "docker" {
		t.Errorf("expected environment type docker, got %s", cfg.Environment.Type)
	}

	if cfg.Environment.PreserveEnv != models.PreserveOnFailure {
		t.Errorf("expected preserve_env on_failure, got %s", cfg.Environment.PreserveEnv)
	}
    
    // Check overrides
    if cfg.Environment.OverrideCPUs == nil || *cfg.Environment.OverrideCPUs != 4 {
        t.Errorf("expected override_cpus 4, got %v", cfg.Environment.OverrideCPUs)
    }
    
    if cfg.Environment.OverrideMemoryMB == nil || *cfg.Environment.OverrideMemoryMB != 8192 {
        t.Errorf("expected override_memory_mb 8192, got %v", cfg.Environment.OverrideMemoryMB)
    }
    
    if cfg.Environment.OverrideStorageMB == nil || *cfg.Environment.OverrideStorageMB != 16384 {
        t.Errorf("expected override_storage_mb 16384, got %v", cfg.Environment.OverrideStorageMB)
    }

	if len(cfg.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(cfg.Agents))
	}

	if !cfg.Agents[0].IsOracle() {
		t.Error("expected first agent to be oracle")
	}

	if cfg.Agents[1].Name != "custom-agent" {
		t.Errorf("expected second agent name custom-agent, got %s", cfg.Agents[1].Name)
	}
}

func TestDefaultJobConfig(t *testing.T) {
	cfg := config.DefaultJobConfig()

	if cfg.JobsDir != "jobs" {
		t.Errorf("expected default jobs_dir 'jobs', got %s", cfg.JobsDir)
	}

	if cfg.NAttempts != 1 {
		t.Errorf("expected default n_attempts 1, got %d", cfg.NAttempts)
	}

	if cfg.InstructionPath != "/tmp/instruction.md" {
		t.Errorf("expected default instruction_path /tmp/instruction.md, got %s", cfg.InstructionPath)
	}

	if cfg.Environment.Type != "docker" {
		t.Errorf("expected default environment type docker, got %s", cfg.Environment.Type)
	}

	if cfg.Environment.PreserveEnv != models.PreserveNever {
		t.Errorf("expected default preserve_env never, got %s", cfg.Environment.PreserveEnv)
	}
}
