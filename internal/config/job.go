package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
	"github.com/spachava753/rollout/internal/models"
)

// DefaultJobConfig returns a JobConfig with default values.
func DefaultJobConfig() models.JobConfig {
	return models.JobConfig{
		JobsDir:           "jobs",
		NAttempts:         1,
		NConcurrentTrials: 1,
		TimeoutMultiplier: 1.0,
		InstructionPath:   "/tmp/instruction.md",
		Retry: models.RetryConfig{
			MaxAttempts:    3,
			InitialDelayMs: 1000,
			MaxDelayMs:     30000,
			Multiplier:     2.0,
		},
		Environment: models.JobEnvironmentConfig{
			Type:        "docker",
			PreserveEnv: models.PreserveNever,
		},
	}
}

// LoadJobConfig loads and parses a job.yaml file.
func LoadJobConfig(path string) (models.JobConfig, error) {
	cfg := DefaultJobConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading job config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing job config: %w", err)
	}

	// Validate dataset refs
	for i, ref := range cfg.Datasets {
		hasPath := ref.Path != nil && *ref.Path != ""
		hasRegistry := ref.Registry != nil
		if !hasPath && !hasRegistry {
			return cfg, fmt.Errorf("dataset[%d]: must specify either 'path' or 'registry'", i)
		}
		if hasPath && hasRegistry {
			return cfg, fmt.Errorf("dataset[%d]: cannot specify both 'path' and 'registry'", i)
		}
	}

	// Apply defaults for missing values
	if cfg.JobsDir == "" {
		cfg.JobsDir = "jobs"
	}
	if cfg.NAttempts == 0 {
		cfg.NAttempts = 1
	}
	if cfg.NConcurrentTrials == 0 {
		cfg.NConcurrentTrials = 1
	}
	if cfg.TimeoutMultiplier == 0 {
		cfg.TimeoutMultiplier = 1.0
	}
	if cfg.InstructionPath == "" {
		cfg.InstructionPath = "/tmp/instruction.md"
	}
	if cfg.Environment.Type == "" {
		cfg.Environment.Type = "docker"
	}
	if cfg.Environment.PreserveEnv == "" {
		cfg.Environment.PreserveEnv = models.PreserveNever
	}

	return cfg, nil
}
