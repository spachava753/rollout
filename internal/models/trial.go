package models

import "time"

// Trial represents a single agent attempt at a task.
type Trial struct {
	ID        string // unique identifier
	Task      Task
	Agent     Agent
	Dataset   string
	Attempt   int
	OutputDir string // path to trial output directory
}

// TrialSpec is the input specification for creating a trial.
type TrialSpec struct {
	Task    Task
	Agent   Agent
	Dataset string
	Attempt int
}

// TrialResult contains the outcome of a trial execution.
type TrialResult struct {
	TaskName        string      `json:"task_name"`
	DatasetName     string      `json:"dataset_name"`
	AgentName       string      `json:"agent_name"`
	Attempt         int         `json:"attempt"`
	TaskGitCommitID *string     `json:"task_git_commit_id"`
	Reward          *float64    `json:"reward"`
	Cost            float64     `json:"cost"`
	Error           *TrialError `json:"error"`
	Durations       Durations   `json:"durations"`
	Timestamps      Timestamps  `json:"timestamps"`
}

type TrialError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
}

type Durations struct {
	TotalSec            float64  `json:"total_sec"`
	EnvironmentSetupSec *float64 `json:"environment_setup_sec"`
	AgentSetupSec       *float64 `json:"agent_setup_sec"`
	AgentExecutionSec   *float64 `json:"agent_execution_sec"`
	VerifierSec         *float64 `json:"verifier_sec"`
}

type Timestamps struct {
	StartedAt                 time.Time  `json:"started_at"`
	EnvironmentSetupStartedAt time.Time  `json:"environment_setup_started_at"`
	EnvironmentSetupEndedAt   time.Time  `json:"environment_setup_ended_at"`
	AgentSetupStartedAt       time.Time  `json:"agent_setup_started_at"`
	AgentSetupEndedAt         time.Time  `json:"agent_setup_ended_at"`
	AgentExecutionStartedAt   time.Time  `json:"agent_execution_started_at"`
	AgentExecutionEndedAt     time.Time  `json:"agent_execution_ended_at"`
	VerifierStartedAt         *time.Time `json:"verifier_started_at"`
	VerifierEndedAt           *time.Time `json:"verifier_ended_at"`
	EndedAt                   time.Time  `json:"ended_at"`
}
