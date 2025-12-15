package config

import (
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
	"github.com/spachava753/rollout/internal/models"
)

// DefaultTaskConfig returns a TaskConfig with default values.
func DefaultTaskConfig() models.TaskConfig {
	return models.TaskConfig{
		Version: "1.0",
		Verifier: models.VerifierConfig{
			TimeoutSec: 600.0,
		},
		Agent: models.AgentTaskConfig{
			InstallTimeoutSec: 300.0,
			TimeoutSec:        600.0,
		},
		Env: models.EnvironmentConfig{
			BuildTimeoutSec: 600.0,
			CPUs:            "1",
			Memory:          "2G",
			Storage:         "10G",
		},
	}
}

// LoadTaskConfig loads and parses a task.toml file from the given filesystem.
func LoadTaskConfig(fsys fs.FS) (models.TaskConfig, error) {
	cfg := DefaultTaskConfig()

	data, err := fs.ReadFile(fsys, "task.toml")
	if err != nil {
		return cfg, fmt.Errorf("reading task.toml: %w", err)
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("parsing task.toml: %w", err)
	}

	return cfg, nil
}
