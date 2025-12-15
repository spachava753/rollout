package dataset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spachava753/rollout/internal/models"
	"github.com/spachava753/rollout/internal/task"
)

// Loader loads datasets from local paths.
type Loader struct {
	taskLoader *task.Loader
}

// NewLoader creates a new dataset loader.
func NewLoader() *Loader {
	return &Loader{
		taskLoader: task.NewLoader(),
	}
}

// LoadFromPath loads all tasks from a local dataset directory.
func (l *Loader) LoadFromPath(ctx context.Context, datasetPath string) (*models.Dataset, error) {
	absPath, err := filepath.Abs(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading dataset directory: %w", err)
	}

	var tasks []models.Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskPath := filepath.Join(absPath, entry.Name())
		t, err := l.taskLoader.LoadTask(ctx, taskPath)
		if err != nil {
			return nil, fmt.Errorf("loading task %s: %w", entry.Name(), err)
		}

		if err := l.taskLoader.ValidateTask(t); err != nil {
			return nil, fmt.Errorf("validating task %s: %w", entry.Name(), err)
		}

		tasks = append(tasks, *t)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks found in dataset %s", absPath)
	}

	name := filepath.Base(absPath)
	return &models.Dataset{
		Name:  name,
		Tasks: tasks,
	}, nil
}
