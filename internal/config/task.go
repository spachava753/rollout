package config

import (
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
	"github.com/spachava753/rollout/internal/models"
	"github.com/spachava753/rollout/internal/util"
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
			CPUs:            1,
			MemoryMB:        2048,  // 2G
			StorageMB:       10240, // 10G
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

	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("parsing task.toml: %w", err)
	}

	// Handle legacy 'memory' field if 'memory_mb' is not explicitly set
	if !md.IsDefined("environment", "memory_mb") && md.IsDefined("environment", "memory") {
		mb, err := util.ParseMemory(cfg.Env.Memory)
		if err != nil {
			return cfg, fmt.Errorf("parsing memory %q: %w", cfg.Env.Memory, err)
		}
		cfg.Env.MemoryMB = mb
	}

	// Handle legacy 'storage' field if 'storage_mb' is not explicitly set
	if !md.IsDefined("environment", "storage_mb") && md.IsDefined("environment", "storage") {
		mb, err := util.ParseMemory(cfg.Env.Storage)
		if err != nil {
			return cfg, fmt.Errorf("parsing storage %q: %w", cfg.Env.Storage, err)
		}
		cfg.Env.StorageMB = mb
	}

	return cfg, nil
}
