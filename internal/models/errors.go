package models

// ErrorType identifies the category of error that occurred.
type ErrorType string

const (
	// Environment build phase
	ErrEnvironmentBuildFailed              ErrorType = "environment_build_failed"
	ErrEnvironmentBuildTimeout             ErrorType = "environment_build_timeout"
	ErrEnvironmentImagePullFailed          ErrorType = "environment_image_pull_failed"

	// Environment start phase
	ErrEnvironmentStartFailed              ErrorType = "environment_start_failed"
	ErrEnvironmentResourceAllocationFailed ErrorType = "environment_resource_allocation_failed"

	// Agent install phase
	ErrAgentInstallFailed   ErrorType = "agent_install_failed"
	ErrAgentInstallTimeout  ErrorType = "agent_install_timeout"

	// Agent execution phase
	ErrAgentExecutionFailed  ErrorType = "agent_execution_failed"
	ErrAgentExecutionTimeout ErrorType = "agent_execution_timeout"

	// Verification phase
	ErrVerifierFailed        ErrorType = "verifier_failed"
	ErrVerifierTimeout       ErrorType = "verifier_timeout"
	ErrVerifierRewardMissing ErrorType = "verifier_reward_missing"
	ErrVerifierRewardInvalid ErrorType = "verifier_reward_invalid"

	// Teardown phase
	ErrEnvironmentTeardownFailed ErrorType = "environment_teardown_failed"

	// Pre-execution
	ErrTaskInvalid  ErrorType = "task_invalid"
	ErrTaskNotFound ErrorType = "task_not_found"

	// Catch-all
	ErrInternalError ErrorType = "internal_error"
)
