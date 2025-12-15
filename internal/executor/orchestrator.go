package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/dataset"
	"github.com/spachava753/rollout/internal/environment"
	"github.com/spachava753/rollout/internal/environment/docker"
	"github.com/spachava753/rollout/internal/models"
)

// JobOrchestrator coordinates the execution of all trials in a job.
type JobOrchestrator struct {
	cfg      models.JobConfig
	provider environment.Provider
}

// NewJobOrchestrator creates a new job orchestrator.
func NewJobOrchestrator(cfg models.JobConfig) (*JobOrchestrator, error) {
	var provider environment.Provider
	switch cfg.Environment.Type {
	case "docker":
		provider = docker.NewProvider()
	default:
		return nil, fmt.Errorf("unsupported environment type: %s", cfg.Environment.Type)
	}

	return &JobOrchestrator{
		cfg:      cfg,
		provider: provider,
	}, nil
}

// Run executes all trials defined by the job configuration.
func (o *JobOrchestrator) Run(ctx context.Context) (*models.JobResult, error) {
	startTime := time.Now()

	// Load datasets
	loader := dataset.NewLoader()
	var datasets []models.Dataset

	for _, ref := range o.cfg.Datasets {
		if ref.Path != nil {
			ds, err := loader.LoadFromPath(ctx, *ref.Path)
			if err != nil {
				return nil, fmt.Errorf("loading dataset from path %s: %w", *ref.Path, err)
			}
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

	// Create job output directory
	jobName := time.Now().Format("2006-01-02__15-04-05")
	if o.cfg.Name != nil {
		jobName = *o.cfg.Name
	}
	jobDir := filepath.Join(o.cfg.JobsDir, jobName)
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

	// Execute trials
	executor := NewTrialExecutor(o.cfg.InstructionPath, o.cfg.TimeoutMultiplier)
	var results []*models.TrialResult

	for _, trial := range trials {
		// Create output directory
		os.MkdirAll(trial.OutputDir, 0755)

		result, err := executor.Execute(ctx, trial, o.provider)
		if err != nil {
			return nil, fmt.Errorf("executing trial %s: %w", trial.ID, err)
		}

		// Save trial result
		resultJSON, _ := json.MarshalIndent(result, "", "  ")
		os.WriteFile(filepath.Join(trial.OutputDir, "result.json"), resultJSON, 0644)

		// Write error file if applicable
		if result.Error != nil {
			os.WriteFile(filepath.Join(trial.OutputDir, "error.txt"), []byte(result.Error.Message), 0644)
		}

		results = append(results, result)
	}

	// Aggregate results
	jobResult := o.aggregateResults(jobName, results, startTime)

	// Save job result
	jobResultJSON, _ := json.MarshalIndent(jobResult, "", "  ")
	os.WriteFile(filepath.Join(jobDir, "result.json"), jobResultJSON, 0644)

	return jobResult, nil
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
		// Track per-agent stats
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

	// Calculate pass rate (reward == 1.0)
	var passCount int
	for _, r := range results {
		if r.Reward != nil && *r.Reward == 1.0 {
			passCount++
		}
	}
	if jr.CompletedTrials > 0 {
		jr.PassRate = float64(passCount) / float64(jr.CompletedTrials)
	}

	// Build per-agent summaries
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

// RunFromConfig loads a job config file and executes the job.
func RunFromConfig(ctx context.Context, configPath string) (*models.JobResult, error) {
	cfg, err := config.LoadJobConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading job config: %w", err)
	}

	orchestrator, err := NewJobOrchestrator(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating orchestrator: %w", err)
	}

	return orchestrator.Run(ctx)
}
