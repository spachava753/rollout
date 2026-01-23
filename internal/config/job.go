package config

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"

	"github.com/spachava753/rollout/internal/models"
	"gopkg.in/yaml.v3"
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

		// Validate task filter
		if err := validateTaskFilter(i, ref.Tasks); err != nil {
			return cfg, err
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


// taskNamePattern validates task names: start with alphanumeric, contain only alphanumeric, underscore, hyphen
var taskNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// validateTaskFilter validates task names in a dataset's task filter.
// Returns an error if:
// - Tasks is non-nil but empty
// - Any task name doesn't match the valid pattern
// Logs a warning for duplicate task names but does not return an error.
func validateTaskFilter(datasetIdx int, tasks []string) error {
	// nil is valid (means all tasks)
	if tasks == nil {
		return nil
	}

	// Empty slice is invalid when explicitly specified
	if len(tasks) == 0 {
		return fmt.Errorf("dataset[%d]: tasks cannot be empty when specified", datasetIdx)
	}

	// Track seen task names for duplicate detection
	seen := make(map[string]bool)

	for i, taskName := range tasks {
		// Check for duplicates
		if seen[taskName] {
			slog.Warn("duplicate task name in filter",
				"dataset_index", datasetIdx,
				"task_name", taskName)
			continue
		}
		seen[taskName] = true

		// Validate task name pattern
		if !taskNamePattern.MatchString(taskName) {
			return fmt.Errorf("dataset[%d].tasks[%d]: invalid task name %q (must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$)", datasetIdx, i, taskName)
		}
	}

	return nil
}


// ValidateTaskNames validates a slice of task names using the same rules as job.yaml task filters.
// Returns an error if:
// - tasks is non-nil but empty
// - Any task name doesn't match the valid pattern (alphanumeric start, alphanumeric/underscore/hyphen)
// This is intended for CLI flag validation where dataset index is not relevant.
func ValidateTaskNames(tasks []string) error {
	if tasks == nil {
		return nil
	}

	if len(tasks) == 0 {
		return fmt.Errorf("--tasks cannot be empty when specified")
	}

	for i, taskName := range tasks {
		if !taskNamePattern.MatchString(taskName) {
			return fmt.Errorf("--tasks[%d]: invalid task name %q (must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$)", i, taskName)
		}
	}

	return nil
}
