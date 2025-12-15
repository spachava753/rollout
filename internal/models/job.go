package models

import "time"

// PreservePolicy controls environment cleanup behavior.
type PreservePolicy string

const (
	PreserveNever     PreservePolicy = "never"
	PreserveAlways    PreservePolicy = "always"
	PreserveOnFailure PreservePolicy = "on_failure"
)

// JobConfig represents the parsed job.yaml configuration.
type JobConfig struct {
	Name              *string              `yaml:"name,omitempty" json:"name,omitempty"`
	JobsDir           string               `yaml:"jobs_dir" json:"jobs_dir"`
	NAttempts         int                  `yaml:"n_attempts" json:"n_attempts"`
	NConcurrentTrials int                  `yaml:"n_concurrent_trials" json:"n_concurrent_trials"`
	TimeoutMultiplier float64              `yaml:"timeout_multiplier" json:"timeout_multiplier"`
	Retry             RetryConfig          `yaml:"retry,omitempty" json:"retry,omitempty"`
	LogLevel          string               `yaml:"log_level,omitempty" json:"log_level,omitempty"`
	InstructionPath   string               `yaml:"instruction_path" json:"instruction_path"`
	Environment       JobEnvironmentConfig `yaml:"environment" json:"environment"`
	Verifier          JobVerifierConfig    `yaml:"verifier,omitempty" json:"verifier,omitempty"`
	Metrics           []MetricConfig       `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Agents            []Agent              `yaml:"agents" json:"agents"`
	Datasets          []DatasetRef         `yaml:"datasets" json:"datasets"`
}

type RetryConfig struct {
	MaxAttempts    int     `yaml:"max_attempts" json:"max_attempts"`
	InitialDelayMs int     `yaml:"initial_delay_ms" json:"initial_delay_ms"`
	MaxDelayMs     int     `yaml:"max_delay_ms" json:"max_delay_ms"`
	Multiplier     float64 `yaml:"multiplier" json:"multiplier"`
}

type JobEnvironmentConfig struct {
	Type            string         `yaml:"type" json:"type"`
	ForceBuild      bool           `yaml:"force_build" json:"force_build"`
	PreserveEnv     PreservePolicy `yaml:"preserve_env" json:"preserve_env"`
	ProviderConfig  map[string]any `yaml:"provider_config,omitempty" json:"provider_config,omitempty"`
	OverrideCPUs    *string        `yaml:"override_cpus,omitempty" json:"override_cpus,omitempty"`
	OverrideMemory  *string        `yaml:"override_memory,omitempty" json:"override_memory,omitempty"`
	OverrideStorage *string        `yaml:"override_storage,omitempty" json:"override_storage,omitempty"`
}

type JobVerifierConfig struct {
	OverrideTimeoutSec *float64 `yaml:"override_timeout_sec,omitempty" json:"override_timeout_sec,omitempty"`
	MaxTimeoutSec      *float64 `yaml:"max_timeout_sec,omitempty" json:"max_timeout_sec,omitempty"`
	Disable            bool     `yaml:"disable" json:"disable"`
}

type MetricConfig struct {
	Type string `yaml:"type" json:"type"`
}

// DatasetRef specifies how to load a dataset.
type DatasetRef struct {
	Path     *string      `yaml:"path,omitempty" json:"path,omitempty"`
	Registry *RegistryRef `yaml:"registry,omitempty" json:"registry,omitempty"`
	Name     string       `yaml:"name,omitempty" json:"name,omitempty"`
	Version  string       `yaml:"version,omitempty" json:"version,omitempty"`
}

type RegistryRef struct {
	Path *string `yaml:"path,omitempty" json:"path,omitempty"`
	URL  *string `yaml:"url,omitempty" json:"url,omitempty"`
}

// Dataset represents a collection of tasks.
type Dataset struct {
	Name    string
	Version string
	Tasks   []Task
}

// JobResult contains aggregate metrics across all trials.
type JobResult struct {
	JobName          string                  `json:"job_name"`
	Cancelled        bool                    `json:"cancelled"`
	TotalTrials      int                     `json:"total_trials"`
	CompletedTrials  int                     `json:"completed_trials"`
	FailedTrials     int                     `json:"failed_trials"`
	SkippedTrials    int                     `json:"skipped_trials"`
	PassRate         float64                 `json:"pass_rate"`
	MeanReward       float64                 `json:"mean_reward"`
	TotalCost        float64                 `json:"total_cost"`
	TotalDurationSec float64                 `json:"total_duration_sec"`
	StartedAt        time.Time               `json:"started_at"`
	EndedAt          time.Time               `json:"ended_at"`
	Agents           map[string]AgentSummary `json:"agents"`
	Results          []TrialSummary          `json:"results"`
}

type AgentSummary struct {
	TotalTrials     int     `json:"total_trials"`
	CompletedTrials int     `json:"completed_trials"`
	FailedTrials    int     `json:"failed_trials"`
	PassRate        float64 `json:"pass_rate"`
	MeanReward      float64 `json:"mean_reward"`
	TotalCost       float64 `json:"total_cost"`
}

type TrialSummary struct {
	TaskName    string   `json:"task_name"`
	DatasetName string   `json:"dataset_name"`
	AgentName   string   `json:"agent_name"`
	Attempt     int      `json:"attempt"`
	Reward      *float64 `json:"reward"`
}
