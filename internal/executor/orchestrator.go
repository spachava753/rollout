package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/dataset"
	"github.com/spachava753/rollout/internal/environment"
	"github.com/spachava753/rollout/internal/environment/docker"
	"github.com/spachava753/rollout/internal/environment/apple"
	"github.com/spachava753/rollout/internal/environment/modal"
	"github.com/spachava753/rollout/internal/models"
)

// TrialExecutor executes a single trial and returns the result.
type TrialExecutor interface {
	Execute(ctx context.Context, trial models.Trial, provider environment.Provider) (*models.TrialResult, error)
}

// NewTrialExecutorFunc creates a TrialExecutor from a JobConfig.
type NewTrialExecutorFunc func(cfg models.JobConfig) TrialExecutor

// JobOrchestrator coordinates the execution of all trials in a job.
type JobOrchestrator struct {
	cfg         models.JobConfig
	provider    environment.Provider
	newExecutor NewTrialExecutorFunc
}

// NewJobOrchestrator creates a new job orchestrator.
func NewJobOrchestrator(cfg models.JobConfig, executorFactory NewTrialExecutorFunc) (*JobOrchestrator, error) {
	var provider environment.Provider
	switch cfg.Environment.Type {
	case "docker":
		provider = docker.NewProvider()
		slog.Debug("initialized docker environment provider")
	case "apple":
		appleCfg := apple.ParseProviderConfig(cfg.Environment.ProviderConfig)
		var err error
		provider, err = apple.NewProvider(appleCfg)
		if err != nil {
			return nil, fmt.Errorf("creating apple provider: %w", err)
		}
		slog.Debug("initialized apple environment provider")
	case "modal":
		modalCfg := modal.ParseProviderConfig(cfg.Environment.ProviderConfig)
		var err error
		provider, err = modal.NewProvider(modalCfg)
		if err != nil {
			return nil, fmt.Errorf("creating modal provider: %w", err)
		}
		slog.Debug("initialized modal environment provider")
	default:
		return nil, fmt.Errorf("unsupported environment type: %s", cfg.Environment.Type)
	}

	return &JobOrchestrator{
		cfg:         cfg,
		provider:    provider,
		newExecutor: executorFactory,
	}, nil
}

// Run executes all trials defined by the job configuration.
func (o *JobOrchestrator) Run(ctx context.Context) (*models.JobResult, error) {
	startTime := time.Now()

	// Load datasets
	slog.Info("loading datasets", "count", len(o.cfg.Datasets))
	loader := dataset.NewLoader()
	var datasets []models.Dataset

	for _, ref := range o.cfg.Datasets {
		if ref.Path != nil {
			slog.Debug("loading dataset from path", "path", *ref.Path)
			ds, err := loader.LoadFromPath(ctx, *ref.Path)
			if err != nil {
				return nil, fmt.Errorf("loading dataset from path %s: %w", *ref.Path, err)
			}
			slog.Info("loaded dataset", "name", ds.Name, "tasks", len(ds.Tasks))
			datasets = append(datasets, *ds)
		} else if ref.Registry != nil {
			slog.Debug("loading dataset from registry", "name", ref.Name, "version", ref.Version)
			ds, err := loader.LoadFromRegistry(ctx, *ref.Registry, ref.Name, ref.Version)
			if err != nil {
				return nil, fmt.Errorf("loading dataset %s from registry: %w", ref.Name, err)
			}
			slog.Info("loaded dataset", "name", ds.Name, "version", ds.Version, "tasks", len(ds.Tasks))
			datasets = append(datasets, *ds)
		}
	}

	// Generate trials (Cartesian product of agents × tasks × attempts)
	var trials []models.Trial
	for _, agent := range o.cfg.Agents {
		for _, ds := range datasets {
			for _, task := range ds.Tasks {
				for attempt := 1; attempt <= o.cfg.NAttempts; attempt++ {
					trialID := fmt.Sprintf("%s__%s__%s__%d", agent.Name, ds.Name, task.Name, attempt)
					outputDir := filepath.Join(o.cfg.JobsDir, agent.Name, ds.Name, fmt.Sprintf("%s__%d", task.Name, attempt))

					trials = append(trials, models.Trial{
						ID:        trialID,
						Task:      task,
						Agent:     agent,
						Dataset:   ds.Name,
						Attempt:   attempt,
						OutputDir: outputDir,
					})
				}
			}
		}
	}

	slog.Info("generated trials",
		"total", len(trials),
		"agents", len(o.cfg.Agents),
		"attempts_per_task", o.cfg.NAttempts)

	// Create job output directory
	jobName := time.Now().Format("2006-01-02__15-04-05")
	if o.cfg.Name != nil {
		jobName = *o.cfg.Name
	}
	jobDir := filepath.Join(o.cfg.JobsDir, jobName)

	if _, err := os.Stat(jobDir); err == nil {
		return nil, fmt.Errorf("job directory already exists: %s (will not overwrite existing results)", jobDir)
	}

	slog.Debug("creating job output directory", "path", jobDir)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return nil, fmt.Errorf("creating job directory: %w", err)
	}

	// Update trial output dirs to include job name
	for i := range trials {
		trials[i].OutputDir = filepath.Join(jobDir, trials[i].Agent.Name, trials[i].Dataset, fmt.Sprintf("%s__%d", trials[i].Task.Name, trials[i].Attempt))
	}

	// Save job config
	cfgJSON, _ := json.MarshalIndent(o.cfg, "", "  ")
	os.WriteFile(filepath.Join(jobDir, "config.json"), cfgJSON, 0644)

	// Check that no trial output directories already exist
	for _, trial := range trials {
		if _, err := os.Stat(trial.OutputDir); err == nil {
			return nil, fmt.Errorf("trial output directory already exists: %s (will not overwrite existing results)", trial.OutputDir)
		}
	}

	// Execute trials concurrently
	nWorkers := o.cfg.NConcurrentTrials
	if nWorkers <= 0 {
		nWorkers = 1
	}
	if nWorkers > len(trials) {
		nWorkers = len(trials)
	}

	slog.Info("starting trial execution",
		"workers", nWorkers,
		"total_trials", len(trials))

	results, skipped := o.runConcurrent(ctx, trials, nWorkers)

	// Aggregate results
	jobResult := o.aggregateResults(jobName, results, startTime)
	jobResult.SkippedTrials = skipped
	if skipped > 0 {
		jobResult.Cancelled = true
		slog.Info("job cancelled", "completed", len(results), "skipped", skipped)
	}

	// Save job result
	slog.Debug("writing job result", "path", filepath.Join(jobDir, "result.json"))
	jobResultJSON, _ := json.MarshalIndent(jobResult, "", "  ")
	os.WriteFile(filepath.Join(jobDir, "result.json"), jobResultJSON, 0644)

	slog.Info("job completed",
		"duration", time.Since(startTime).Round(time.Second),
		"completed", jobResult.CompletedTrials,
		"failed", jobResult.FailedTrials,
		"pass_rate", fmt.Sprintf("%.2f%%", jobResult.PassRate*100))

	return jobResult, nil
}

// runConcurrent executes trials using a fan-out/fan-in pattern.
// Returns collected results and count of skipped trials.
func (o *JobOrchestrator) runConcurrent(ctx context.Context, trials []models.Trial, nWorkers int) ([]*models.TrialResult, int) {
	trialChan := make(chan models.Trial) // unbuffered
	resultChan := make(chan *models.TrialResult, len(trials))

	var wg sync.WaitGroup

	// Start workers
	for range nWorkers {
		wg.Go(func() {
			executor := o.newExecutor(o.cfg)

			for trial := range trialChan {
				os.MkdirAll(trial.OutputDir, 0755)

				result, err := executor.Execute(ctx, trial, o.provider)
				if err != nil {
					slog.Error("trial execution error",
						"task", trial.Task.Name,
						"agent", trial.Agent.Name,
						"error", err)
					result = &models.TrialResult{
						TaskName:    trial.Task.Name,
						DatasetName: trial.Dataset,
						AgentName:   trial.Agent.Name,
						Attempt:     trial.Attempt,
						Error: &models.TrialError{
							Type:    models.ErrInternalError,
							Message: err.Error(),
						},
					}
				}

				resultJSON, _ := json.MarshalIndent(result, "", "  ")
				os.WriteFile(filepath.Join(trial.OutputDir, "result.json"), resultJSON, 0644)

				if result.Error != nil {
					os.WriteFile(filepath.Join(trial.OutputDir, "error.txt"), []byte(result.Error.Message), 0644)
				}

				resultChan <- result
			}
		})
	}

	// Feeder goroutine: sends trials to workers, respects context cancellation
	fed := 0
	go func() {
		defer close(trialChan)
		for _, trial := range trials {
			select {
			case <-ctx.Done():
				slog.Debug("stopping trial feeder due to context cancellation")
				return
			case trialChan <- trial:
				fed++
			}
		}
	}()

	// Wait for workers to finish, then close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var results []*models.TrialResult
	for result := range resultChan {
		results = append(results, result)
	}

	skipped := max(len(trials)-len(results), 0)

	return results, skipped
}

func (o *JobOrchestrator) aggregateResults(jobName string, results []*models.TrialResult, startTime time.Time) *models.JobResult {
	jr := &models.JobResult{
		JobName:     jobName,
		TotalTrials: len(results),
		StartedAt:   startTime,
		EndedAt:     time.Now(),
		Agents:      make(map[string]models.AgentSummary),
		Results:     make([]models.TrialSummary, 0, len(results)),
	}

	jr.TotalDurationSec = jr.EndedAt.Sub(jr.StartedAt).Seconds()

	var totalReward float64
	var rewardCount int

	agentData := make(map[string]struct {
		total     int
		completed int
		failed    int
		rewards   []float64
		cost      float64
	})

	for _, r := range results {
		ad := agentData[r.AgentName]
		ad.total++
		ad.cost += r.Cost
		jr.TotalCost += r.Cost

		if r.Error != nil {
			jr.FailedTrials++
			ad.failed++
		} else if r.Reward != nil {
			jr.CompletedTrials++
			ad.completed++
			ad.rewards = append(ad.rewards, *r.Reward)
			totalReward += *r.Reward
			rewardCount++
		}

		agentData[r.AgentName] = ad

		jr.Results = append(jr.Results, models.TrialSummary{
			TaskName:    r.TaskName,
			DatasetName: r.DatasetName,
			AgentName:   r.AgentName,
			Attempt:     r.Attempt,
			Reward:      r.Reward,
		})
	}

	if rewardCount > 0 {
		jr.MeanReward = totalReward / float64(rewardCount)
	}

	var passCount int
	for _, r := range results {
		if r.Reward != nil && *r.Reward == 1.0 {
			passCount++
		}
	}
	if jr.CompletedTrials > 0 {
		jr.PassRate = float64(passCount) / float64(jr.CompletedTrials)
	}

	for agentName, ad := range agentData {
		var meanReward float64
		if len(ad.rewards) > 0 {
			for _, r := range ad.rewards {
				meanReward += r
			}
			meanReward /= float64(len(ad.rewards))
		}

		var passRate float64
		var passes int
		for _, r := range ad.rewards {
			if r == 1.0 {
				passes++
			}
		}
		if ad.completed > 0 {
			passRate = float64(passes) / float64(ad.completed)
		}

		jr.Agents[agentName] = models.AgentSummary{
			TotalTrials:     ad.total,
			CompletedTrials: ad.completed,
			FailedTrials:    ad.failed,
			PassRate:        passRate,
			MeanReward:      meanReward,
			TotalCost:       ad.cost,
		}
	}

	return jr
}

// DefaultTrialExecutorFunc creates a default trial executor.
func DefaultTrialExecutorFunc(cfg models.JobConfig) TrialExecutor {
	return NewTrialExecutor(cfg.InstructionPath, cfg.TimeoutMultiplier, cfg.Verifier, cfg.Environment)
}

// RunFromConfig loads a job config file and executes the job.
func RunFromConfig(ctx context.Context, configPath string) (*models.JobResult, error) {
	slog.Info("loading job config", "path", configPath)
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading job config: %w", err)
	}

	// Configure logging based on job config
	configureLogging(cfg.LogLevel)

	orchestrator, err := NewJobOrchestrator(cfg, DefaultTrialExecutorFunc)
	if err != nil {
		return nil, fmt.Errorf("creating orchestrator: %w", err)
	}

	return orchestrator.Run(ctx)
}

// configureLogging sets up slog based on the log level from job config.
func configureLogging(level string) {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
	slog.Debug("logging configured", "level", level)
}
