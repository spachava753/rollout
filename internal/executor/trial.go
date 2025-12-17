package executor

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
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
	VerifierConfig    models.JobVerifierConfig
	EnvOverrides      models.JobEnvironmentConfig
}

// NewTrialExecutor creates a new trial executor.
func NewTrialExecutor(instructionPath string, timeoutMult float64, verifierCfg models.JobVerifierConfig, envOverrides models.JobEnvironmentConfig) *DefaultTrialExecutor {
	return &DefaultTrialExecutor{
		InstructionPath:   instructionPath,
		TimeoutMultiplier: timeoutMult,
		VerifierConfig:    verifierCfg,
		EnvOverrides:      envOverrides,
	}
}

// Execute runs the trial and returns the result.
func (e *DefaultTrialExecutor) Execute(ctx context.Context, trial models.Trial, provider environment.Provider) (*models.TrialResult, error) {
	logger := slog.With(
		"task", trial.Task.Name,
		"agent", trial.Agent.Name,
		"dataset", trial.Dataset,
		"attempt", trial.Attempt,
	)

	logger.Info("starting trial")
	
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
		
		if result.Error != nil {
			logger.Error("trial failed",
				"error_type", result.Error.Type,
				"error", result.Error.Message,
				"duration", fmt.Sprintf("%.2fs", result.Durations.TotalSec))
		} else {
			logger.Info("trial completed",
				"reward", *result.Reward,
				"duration", fmt.Sprintf("%.2fs", result.Durations.TotalSec))
		}
	}()

	// Phase 1: Environment Setup
	logger.Debug("phase 1: setting up environment")
	result.Timestamps.EnvironmentSetupStartedAt = time.Now()
	env, err = e.setupEnvironment(ctx, trial, provider, logger)
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

	// Phase 6: Teardown (deferred)
	defer func() {
		if env != nil {
			logger.Debug("phase 6: tearing down environment", "env_id", env.ID())
			if err := env.Destroy(context.Background()); err != nil {
				logger.Error("failed to destroy environment", "error", err)
			} else {
				logger.Debug("environment destroyed", "env_id", env.ID())
			}
		}
	}()

	// Copy instruction.md
	logger.Debug("copying instruction.md to container", "dest", e.InstructionPath)
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
	logger.Debug("copying tests directory to container", "dest", "/tests")
	testsDir := filepath.Join(trial.Task.Path, "tests")
	if err := env.CopyTo(ctx, testsDir, "/tests"); err != nil {
		result.Error = &models.TrialError{
			Type:    models.ErrEnvironmentStartFailed,
			Message: fmt.Sprintf("copying tests: %s", err),
		}
		return result, nil
	}

	// Create /logs directories
	logger.Debug("creating log directories in container")
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
	logger.Debug("phase 2: installing agent")
	result.Timestamps.AgentSetupStartedAt = time.Now()
	err = e.installAgent(ctx, trial, env, result, logger)
	result.Timestamps.AgentSetupEndedAt = time.Now()
	installDur := result.Timestamps.AgentSetupEndedAt.Sub(result.Timestamps.AgentSetupStartedAt).Seconds()
	result.Durations.AgentSetupSec = &installDur

	if result.Error != nil {
		return result, nil
	}
	logger.Debug("agent install completed", "duration", fmt.Sprintf("%.2fs", installDur))

	// Phase 3: Agent Execute
	logger.Debug("phase 3: executing agent")
	result.Timestamps.AgentExecutionStartedAt = time.Now()
	err = e.executeAgent(ctx, trial, env, result, logger)
	result.Timestamps.AgentExecutionEndedAt = time.Now()
	execDur := result.Timestamps.AgentExecutionEndedAt.Sub(result.Timestamps.AgentExecutionStartedAt).Seconds()
	result.Durations.AgentExecutionSec = &execDur

	if result.Error != nil {
		return result, nil
	}
	logger.Debug("agent execution completed", "duration", fmt.Sprintf("%.2fs", execDur))

	// Phase 4: Verification
	logger.Debug("phase 4: running verifier")
	now := time.Now()
	result.Timestamps.VerifierStartedAt = &now
	err = e.runVerifier(ctx, trial, env, result, logger)
	endNow := time.Now()
	result.Timestamps.VerifierEndedAt = &endNow
	verifierDur := endNow.Sub(now).Seconds()
	result.Durations.VerifierSec = &verifierDur

	// Phase 5: Collect results (copy /logs)
	logger.Debug("phase 5: collecting results")
	if trial.OutputDir != "" {
		logsDir := filepath.Join(trial.OutputDir, "logs")
		os.MkdirAll(logsDir, 0755)
		logger.Debug("copying logs from container", "src", "/logs", "dest", logsDir)
		env.CopyFrom(ctx, "/logs/.", logsDir)
	}

	result.Cost = env.Cost()
	return result, nil
}

func (e *DefaultTrialExecutor) setupEnvironment(ctx context.Context, trial models.Trial, provider environment.Provider, logger *slog.Logger) (environment.Environment, error) {
	var imageRef string
	var err error

	// Check if a pre-built docker image is specified and force_build is not set
	if trial.Task.Config.Env.DockerImage != nil && !e.EnvOverrides.ForceBuild {
		imageRef = *trial.Task.Config.Env.DockerImage
		logger.Debug("using pre-built image", "image", imageRef)
		if err := provider.PullImage(ctx, imageRef); err != nil {
			logger.Error("image pull failed", "error", err)
			return nil, fmt.Errorf("pulling image: %w", err)
		}
		logger.Debug("image ready", "image_ref", imageRef)
	} else {
		// Build image from Dockerfile
		envDir := filepath.Join(trial.Task.Path, "environment")
		tag := fmt.Sprintf("rollout-%s-%s:%d", trial.Task.Name, trial.Agent.Name, time.Now().UnixNano())

		timeout := time.Duration(trial.Task.Config.Env.BuildTimeoutSec*e.TimeoutMultiplier) * time.Second
		logger.Debug("building image",
			"context_dir", envDir,
			"tag", tag,
			"timeout", timeout)

		imageRef, err = provider.BuildImage(ctx, environment.BuildImageOptions{
			ContextDir: envDir,
			Tag:        tag,
			Timeout:    timeout,
		})
		if err != nil {
			logger.Error("image build failed", "error", err)
			return nil, fmt.Errorf("building image: %w", err)
		}
		logger.Debug("image built successfully", "image_ref", imageRef)
	}

	// Determine Memory and Storage
	memoryMB := trial.Task.Config.Env.MemoryMB
	if e.EnvOverrides.OverrideMemoryMB != nil {
		memoryMB = *e.EnvOverrides.OverrideMemoryMB
	}
	
	storageMB := trial.Task.Config.Env.StorageMB
	if e.EnvOverrides.OverrideStorageMB != nil {
		storageMB = *e.EnvOverrides.OverrideStorageMB
	}
	
	// Determine CPUs
	cpus := trial.Task.Config.Env.CPUs
	if e.EnvOverrides.OverrideCPUs != nil {
		cpus = *e.EnvOverrides.OverrideCPUs
	}

	// Create environment with meaningful name for debugging
	envName := formatEnvironmentName(trial.Dataset, trial.Task.Name, trial.Agent.Name, trial.Attempt)
	logger.Debug("creating environment",
		"name", envName,
		"cpus", cpus,
		"memory_mb", memoryMB,
		"storage_mb", storageMB)
	
	env, err := provider.CreateEnvironment(ctx, environment.CreateEnvironmentOptions{
		Name:      envName,
		ImageRef:  imageRef,
		CPUs:      cpus,
		MemoryMB:  memoryMB,
		StorageMB: storageMB,
		Env:       trial.Agent.Env,
	})
	if err != nil {
		logger.Error("environment creation failed", "error", err)
		return nil, fmt.Errorf("creating environment: %w", err)
	}

	logger.Debug("environment created", "env_id", env.ID())
	return env, nil
}

func (e *DefaultTrialExecutor) installAgent(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult, logger *slog.Logger) error {
	if trial.Agent.IsOracle() {
		// Oracle agent: copy solution
		solDir := filepath.Join(trial.Task.Path, "solution")
		logger.Debug("copying oracle solution to container", "src", solDir, "dest", "/oracle")
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
		logger.Debug("no install script, skipping agent install")
		return nil
	}

	timeout := time.Duration(trial.Task.Config.Agent.InstallTimeoutSec*e.TimeoutMultiplier) * time.Second
	logger.Debug("executing agent install script", "timeout", timeout)
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
			logger.Error("agent install timed out", "timeout", timeout)
			result.Error = &models.TrialError{
				Type:    models.ErrAgentInstallTimeout,
				Message: err.Error(),
			}
		} else {
			logger.Error("agent install failed", "error", err)
			result.Error = &models.TrialError{
				Type:    models.ErrAgentInstallFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		logger.Error("agent install failed", "exit_code", exitCode)
		result.Error = &models.TrialError{
			Type:    models.ErrAgentInstallFailed,
			Message: fmt.Sprintf("install script exited with code %d", exitCode),
		}
		return fmt.Errorf("install failed with exit code %d", exitCode)
	}

	return nil
}

func (e *DefaultTrialExecutor) executeAgent(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult, logger *slog.Logger) error {
	var cmd string
	if trial.Agent.IsOracle() {
		cmd = "bash /oracle/solve.sh"
	} else {
		cmd = trial.Agent.Execute
	}

	if cmd == "" {
		logger.Debug("no execute script, skipping agent execution")
		return nil
	}

	timeout := time.Duration(trial.Task.Config.Agent.TimeoutSec*e.TimeoutMultiplier) * time.Second
	logger.Debug("executing agent command", "timeout", timeout)
	var stdout, stderr bytes.Buffer

	execEnv := make(map[string]string)
	maps.Copy(execEnv, trial.Agent.Env)
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
			logger.Error("agent execution timed out", "timeout", timeout)
			result.Error = &models.TrialError{
				Type:    models.ErrAgentExecutionTimeout,
				Message: err.Error(),
			}
		} else {
			logger.Error("agent execution failed", "error", err)
			result.Error = &models.TrialError{
				Type:    models.ErrAgentExecutionFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		logger.Error("agent execution failed", "exit_code", exitCode)
		result.Error = &models.TrialError{
			Type:    models.ErrAgentExecutionFailed,
			Message: fmt.Sprintf("agent exited with code %d", exitCode),
		}
		return fmt.Errorf("agent failed with exit code %d", exitCode)
	}

	return nil
}


// ComputeVerifierTimeout calculates the effective timeout for the verifier,
// applying override, max ceiling, and multiplier logic.
func (e *DefaultTrialExecutor) ComputeVerifierTimeout(taskTimeoutSec float64) time.Duration {
	timeoutSec := taskTimeoutSec

	// Override takes precedence if set
	if e.VerifierConfig.OverrideTimeoutSec != nil && *e.VerifierConfig.OverrideTimeoutSec > 0 {
		timeoutSec = *e.VerifierConfig.OverrideTimeoutSec
	}

	// Apply multiplier
	timeoutSec *= e.TimeoutMultiplier

	// Apply max ceiling if set
	if e.VerifierConfig.MaxTimeoutSec != nil && *e.VerifierConfig.MaxTimeoutSec > 0 {
		maxSec := *e.VerifierConfig.MaxTimeoutSec * e.TimeoutMultiplier
		if timeoutSec > maxSec {
			timeoutSec = maxSec
		}
	}

	return time.Duration(timeoutSec) * time.Second
}

func (e *DefaultTrialExecutor) runVerifier(ctx context.Context, trial models.Trial, env environment.Environment, result *models.TrialResult, logger *slog.Logger) error {
	timeout := e.ComputeVerifierTimeout(trial.Task.Config.Verifier.TimeoutSec)
	logger.Debug("executing verifier", "timeout", timeout)
	var stdout, stderr bytes.Buffer

	exitCode, err := env.Exec(ctx, "bash /tests/test.sh", &stdout, &stderr, environment.ExecOptions{
		Timeout: timeout,
	})

	// Save verifier logs to container's /logs/verifier
	env.Exec(ctx, fmt.Sprintf("echo %q > /logs/verifier/stdout.txt", stdout.String()), nil, nil, environment.ExecOptions{})
	env.Exec(ctx, fmt.Sprintf("echo %q > /logs/verifier/stderr.txt", stderr.String()), nil, nil, environment.ExecOptions{})

	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			logger.Error("verifier timed out", "timeout", timeout)
			result.Error = &models.TrialError{
				Type:    models.ErrVerifierTimeout,
				Message: err.Error(),
			}
		} else {
			logger.Error("verifier failed", "error", err)
			result.Error = &models.TrialError{
				Type:    models.ErrVerifierFailed,
				Message: err.Error(),
			}
		}
		return err
	}

	if exitCode != 0 {
		logger.Error("verifier failed", "exit_code", exitCode)
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierFailed,
			Message: fmt.Sprintf("verifier exited with code %d", exitCode),
		}
		return fmt.Errorf("verifier failed with exit code %d", exitCode)
	}

	// Read reward file
	logger.Debug("reading reward file")
	var rewardBuf bytes.Buffer
	exitCode, err = env.Exec(ctx, "cat /logs/verifier/reward.txt", &rewardBuf, nil, environment.ExecOptions{})
	if err != nil || exitCode != 0 {
		logger.Error("reward file missing")
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierRewardMissing,
			Message: "reward.txt not found",
		}
		return fmt.Errorf("reward file missing")
	}

	rewardStr := strings.TrimSpace(rewardBuf.String())
	reward, err := strconv.ParseFloat(rewardStr, 64)
	if err != nil {
		logger.Error("invalid reward value", "value", rewardStr)
		result.Error = &models.TrialError{
			Type:    models.ErrVerifierRewardInvalid,
			Message: fmt.Sprintf("invalid reward value: %s", rewardStr),
		}
		return fmt.Errorf("invalid reward: %w", err)
	}

	logger.Debug("reward parsed", "reward", reward)
	result.Reward = &reward
	return nil
}


// formatEnvironmentName creates a human-readable environment name from trial context.
// Format: {dataset}-{task}-{agent}-{attempt}-{timestamp}
// Names are sanitized to be valid across providers (lowercase, alphanumeric + hyphens).
// maxAppNameLength is the maximum length for Modal app names.
// Modal rejects names longer than 64 characters.
const maxAppNameLength = 64

func formatEnvironmentName(dataset, task, agent string, attempt int) string {
	ts := time.Now().Unix()
	name := fmt.Sprintf("%s-%s-%s-%d-%d", dataset, task, agent, attempt, ts)
	return sanitizeEnvName(name)
}

// sanitizeEnvName ensures the name is valid for container/app naming.
// Converts to lowercase, replaces invalid chars with hyphens, removes consecutive hyphens,
// and truncates to maxAppNameLength.
func sanitizeEnvName(name string) string {
	name = strings.ToLower(name)
	var result strings.Builder
	prevHyphen := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			result.WriteRune('-')
			prevHyphen = true
		}
	}
	// Trim leading/trailing hyphens
	sanitized := strings.Trim(result.String(), "-")
	
	// Truncate to max length, avoiding trailing hyphen
	if len(sanitized) > maxAppNameLength {
		sanitized = sanitized[:maxAppNameLength]
		sanitized = strings.TrimRight(sanitized, "-")
	}
	return sanitized
}
