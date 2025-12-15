package executor

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spachava753/rollout/internal/environment"
	"github.com/spachava753/rollout/internal/models"
)

// DefaultTrialExecutor runs a single trial through all phases.
type DefaultTrialExecutor struct {
	InstructionPath   string
	TimeoutMultiplier float64
}

// NewTrialExecutor creates a new trial executor.
func NewTrialExecutor(instructionPath string, timeoutMult float64) *DefaultTrialExecutor {
	return &DefaultTrialExecutor{
		InstructionPath:   instructionPath,
		TimeoutMultiplier: timeoutMult,
	}
}

// Execute runs the trial and returns the result.
func (e *DefaultTrialExecutor) Execute(ctx context.Context, trial models.Trial, provider environment.Provider) (*models.TrialResult, error) {
	result := &models.TrialResult{
		TaskName:        trial.Task.Name,
		DatasetName:     trial.Dataset,
		AgentName:       trial.Agent.Name,
		Attempt:         trial.Attempt,
		TaskGitCommitID: trial.Task.GitCommitID,
		Timestamps: models.Timestamps{
			StartedAt: time.Now(),
		},
	}

	var env environment.Environment
	var err error

	defer func() {
		result.Timestamps.EndedAt = time.Now()
		result.Durations.TotalSec = result.Timestamps.EndedAt.Sub(result.Timestamps.StartedAt).Seconds()
	}()

	// Phase 1: Environment Setup
	result.Timestamps.EnvironmentSetupStartedAt = time.Now()
	env, err = e.setupEnvironment(ctx, trial, provider)
	result.Timestamps.EnvironmentSetupEndedAt = time.Now()
	setupDur := result.Timestamps.EnvironmentSetupEndedAt.Sub(result.Timestamps.EnvironmentSetupStartedAt).Seconds()
	result.Durations.EnvironmentSetupSec = &setupDur

	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrEnvironmentBuildFailed,
			Message: err.Error(),
		}
		return result, nil
	}

	defer func() {
		if env != nil {
			env.Destroy(context.Background())
		}
	}()

	// Copy instruction.md
	instrFile, err := trial.Task.Instruction()
	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrTaskInvalid,
			Message: fmt.Sprintf("reading instruction: %s", err),
		}
		return result, nil
	}
	defer instrFile.Close()

	// Write instruction to temp file then copy
	instrContent, err := fs.ReadFile(trial.Task.FS, "instruction.md")
	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrTaskInvalid,
			Message: fmt.Sprintf("reading instruction: %s", err),
		}
		return result, nil
	}

	tmpInstr, err := os.CreateTemp("", "instruction-*.md")
	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrInternalError,
			Message: fmt.Sprintf("creating temp instruction: %s", err),
		}
		return result, nil
	}
	tmpInstr.Write(instrContent)
	tmpInstr.Close()
	defer os.Remove(tmpInstr.Name())

	if err := env.CopyTo(ctx, tmpInstr.Name(), e.InstructionPath); err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrEnvironmentStartFailed,
			Message: fmt.Sprintf("copying instruction: %s", err),
		}
		return result, nil
	}

	// Copy tests/ directory
	testsDir := filepath.Join(trial.Task.Path, "tests")
	if err := env.CopyTo(ctx, testsDir, "/tests"); err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrEnvironmentStartFailed,
			Message: fmt.Sprintf("copying tests: %s", err),
		}
		return result, nil
	}

	// Create /logs directories
	var createLogsDirs bytes.Buffer
	_, err = env.Exec(ctx, "mkdir -p /logs/verifier /logs/agent", &createLogsDirs, &createLogsDirs, environment.ExecOptions{})
	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrEnvironmentStartFailed,
			Message: fmt.Sprintf("creating log dirs: %s", err),
		}
		return result, nil
	}

	// Phase 2: Agent Install
	result.Timestamps.AgentSetupStartedAt = time.Now()
	err = e.installAgent(ctx, trial, env, result)
	result.Timestamps.AgentSetupEndedAt = time.Now()
	installDur := result.Timestamps.AgentSetupEndedAt.Sub(result.Timestamps.AgentSetupStartedAt).Seconds()
	result.Durations.AgentSetupSec = &installDur

	if result.Error != nil {
		return result, nil
	}

	// Phase 3: Agent Execute
	result.Timestamps.AgentExecutionStartedAt = time.Now()
	err = e.executeAgent(ctx, trial, env, result)
	result.Timestamps.AgentExecutionEndedAt = time.Now()
	execDur := result.Timestamps.AgentExecutionEndedAt.Sub(result.Timestamps.AgentExecutionStartedAt).Seconds()
	result.Durations.AgentExecutionSec = &execDur

	if result.Error != nil {
		return result, nil
	}

	// Phase 4: Verification
	now := time.Now()
	result.Timestamps.VerifierStartedAt = &now
	err = e.runVerifier(ctx, trial, env, result)
	endNow := time.Now()
	result.Timestamps.VerifierEndedAt = &endNow
	verifierDur := endNow.Sub(now).Seconds()
	result.Durations.VerifierSec = &verifierDur

	// Phase 5: Collect results (copy /logs)
	if trial.OutputDir != "" {
		logsDir := filepath.Join(trial.OutputDir, "logs")
		os.MkdirAll(logsDir, 0755)
		env.CopyFrom(ctx, "/logs/.", logsDir)
	}

	result.Cost = env.Cost()
	return result, nil
}

func (e *DefaultTrialExecutor) setupEnvironment(ctx context.Context, trial models.Trial, provider environment.Provider) (environment.Environment, error) {
	// Build image
	envDir := filepath.Join(trial.Task.Path, "environment")
	tag := fmt.Sprintf("rollout-%s-%s:%d", trial.Task.Name, trial.Agent.Name, time.Now().UnixNano())

	timeout := time.Duration(trial.Task.Config.Env.BuildTimeoutSec*e.TimeoutMultiplier) * time.Second
	imageRef, err := provider.BuildImage(ctx, environment.BuildImageOptions{
		ContextDir: envDir,
		Tag:        tag,
		Timeout:    timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("building image: %w", err)
	}

	// Create environment
	env, err := provider.CreateEnvironment(ctx, environment.CreateEnvironmentOptions{
		ImageRef: imageRef,
		CPUs:     trial.Task.Config.Env.CPUs,
		Memory:   trial.Task.Config.Env.Memory,
		Env:      trial.Agent.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("creating environment: %w", err)
	}

	return env, nil
}

func (e *DefaultTrialExecutor) installAgent(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult) error {
	if trial.Agent.IsOracle() {
		// Oracle agent: copy solution
		solDir := filepath.Join(trial.Task.Path, "solution")
		if err := env.CopyTo(ctx, solDir, "/oracle"); err != nil {
			result.Error = &models.TrialError{
				Type:    models.ErrAgentInstallFailed,
				Message: fmt.Sprintf("copying solution: %s", err),
			}
			return err
		}
		return nil
	}

	if trial.Agent.Install == "" {
		return nil
	}

	timeout := time.Duration(trial.Task.Config.Agent.InstallTimeoutSec*e.TimeoutMultiplier) * time.Second
	var stdout, stderr bytes.Buffer

	exitCode, err := env.Exec(ctx, trial.Agent.Install, &stdout, &stderr, environment.ExecOptions{
		Env:     trial.Agent.Env,
		Timeout: timeout,
	})

	// Save install logs
	if trial.OutputDir != "" {
		setupDir := filepath.Join(trial.OutputDir, "setup")
		os.MkdirAll(setupDir, 0755)
		os.WriteFile(filepath.Join(setupDir, "stdout.txt"), stdout.Bytes(), 0644)
		os.WriteFile(filepath.Join(setupDir, "stderr.txt"), stderr.Bytes(), 0644)
	}

	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			result.Error = &models.TrialError{
				Type:    models.ErrAgentInstallTimeout,
				Message: err.Error(),
			}
		} else {
			result.Error = &models.TrialError{
				Type:    models.ErrAgentInstallFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		result.Error = &models.TrialError{
			Type:    models.ErrAgentInstallFailed,
			Message: fmt.Sprintf("install script exited with code %d", exitCode),
		}
		return fmt.Errorf("install failed with exit code %d", exitCode)
	}

	return nil
}

func (e *DefaultTrialExecutor) executeAgent(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult) error {
	var cmd string
	if trial.Agent.IsOracle() {
		cmd = "bash /oracle/solve.sh"
	} else {
		cmd = trial.Agent.Execute
	}

	if cmd == "" {
		return nil
	}

	timeout := time.Duration(trial.Task.Config.Agent.TimeoutSec*e.TimeoutMultiplier) * time.Second
	var stdout, stderr bytes.Buffer

	execEnv := make(map[string]string)
	for k, v := range trial.Agent.Env {
		execEnv[k] = v
	}
	execEnv["ROLLOUT_TASK_INSTRUCTION"] = e.InstructionPath

	exitCode, err := env.Exec(ctx, cmd, &stdout, &stderr, environment.ExecOptions{
		Env:     execEnv,
		Timeout: timeout,
	})

	// Save execution logs
	if trial.OutputDir != "" {
		cmdDir := filepath.Join(trial.OutputDir, "command")
		os.MkdirAll(cmdDir, 0755)
		os.WriteFile(filepath.Join(cmdDir, "stdout.txt"), stdout.Bytes(), 0644)
		os.WriteFile(filepath.Join(cmdDir, "stderr.txt"), stderr.Bytes(), 0644)
	}

	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			result.Error = &models.TrialError{
				Type:    models.ErrAgentExecutionTimeout,
				Message: err.Error(),
			}
		} else {
			result.Error = &models.TrialError{
				Type:    models.ErrAgentExecutionFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		result.Error = &models.TrialError{
			Type:    models.ErrAgentExecutionFailed,
			Message: fmt.Sprintf("agent exited with code %d", exitCode),
		}
		return fmt.Errorf("agent failed with exit code %d", exitCode)
	}

	return nil
}

func (e *DefaultTrialExecutor) runVerifier(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult) error {
	timeout := time.Duration(trial.Task.Config.Verifier.TimeoutSec*e.TimeoutMultiplier) * time.Second
	var stdout, stderr bytes.Buffer

	exitCode, err := env.Exec(ctx, "bash /tests/test.sh", &stdout, &stderr, environment.ExecOptions{
		Timeout: timeout,
	})

	// Save verifier logs to container's /logs/verifier
	env.Exec(ctx, fmt.Sprintf("echo %q > /logs/verifier/stdout.txt", stdout.String()), nil, nil, environment.ExecOptions{})
	env.Exec(ctx, fmt.Sprintf("echo %q > /logs/verifier/stderr.txt", stderr.String()), nil, nil, environment.ExecOptions{})

	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			result.Error = &models.TrialError{
				Type:    models.ErrVerifierTimeout,
				Message: err.Error(),
			}
		} else {
			result.Error = &models.TrialError{
				Type:    models.ErrVerifierFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierFailed,
			Message: fmt.Sprintf("verifier exited with code %d", exitCode),
		}
		return fmt.Errorf("verifier failed with exit code %d", exitCode)
	}

	// Read reward file
	var rewardBuf bytes.Buffer
	exitCode, err = env.Exec(ctx, "cat /logs/verifier/reward.txt", &rewardBuf, nil, environment.ExecOptions{})
	if err != nil || exitCode != 0 {
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierRewardMissing,
			Message: "reward.txt not found",
		}
		return fmt.Errorf("reward file missing")
	}

	rewardStr := strings.TrimSpace(rewardBuf.String())
	reward, err := strconv.ParseFloat(rewardStr, 64)
	if err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierRewardInvalid,
			Message: fmt.Sprintf("invalid reward value: %s", rewardStr),
		}
		return fmt.Errorf("invalid reward: %w", err)
	}

	result.Reward = &reward
	return nil
}
