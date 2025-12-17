package models

import (
	"io/fs"
)

// TaskConfig represents the parsed task.toml configuration.
type TaskConfig struct {
	Version  string            `toml:"version"`
	Source   *string           `toml:"source,omitempty"`
	Metadata map[string]any    `toml:"metadata,omitempty"`
	Verifier VerifierConfig    `toml:"verifier"`
	Agent    AgentTaskConfig   `toml:"agent"`
	Env      EnvironmentConfig `toml:"environment"`
}

type VerifierConfig struct {
	TimeoutSec float64 `toml:"timeout_sec"` // default: 600.0
}

type AgentTaskConfig struct {
	InstallTimeoutSec float64 `toml:"install_timeout_sec"` // default: 300.0
	TimeoutSec        float64 `toml:"timeout_sec"`         // default: 600.0
}

type EnvironmentConfig struct {
	BuildTimeoutSec float64 `toml:"build_timeout_sec"` // default: 600.0
	DockerImage     *string `toml:"docker_image,omitempty"`
	CPUs            int     `toml:"cpus"`    // default: 1
	Memory          string  `toml:"memory,omitempty"`  // Deprecated: use MemoryMB
	Storage         string  `toml:"storage,omitempty"` // Deprecated: use StorageMB
	MemoryMB        int     `toml:"memory_mb,omitempty"`
	StorageMB       int     `toml:"storage_mb,omitempty"`
}

// Task represents a fully loaded task ready for execution.
type Task struct {
	Name        string
	Path        string      // filesystem path to task directory
	FS          fs.FS       // filesystem rooted at task directory
	Config      TaskConfig
	GitCommitID *string     // resolved git SHA, nil if not in git repo
}

// Instruction opens the instruction.md file.
func (t *Task) Instruction() (fs.File, error) {
	return t.FS.Open("instruction.md")
}

// Environment returns the environment subdirectory filesystem.
func (t *Task) Environment() (fs.FS, error) {
	return fs.Sub(t.FS, "environment")
}

// Solution returns the solution subdirectory filesystem.
func (t *Task) Solution() (fs.FS, error) {
	return fs.Sub(t.FS, "solution")
}

// Tests returns the tests subdirectory filesystem.
func (t *Task) Tests() (fs.FS, error) {
	return fs.Sub(t.FS, "tests")
}
