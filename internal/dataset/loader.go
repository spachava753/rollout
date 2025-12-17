package dataset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spachava753/rollout/internal/models"
	"github.com/spachava753/rollout/internal/registry"
	"github.com/spachava753/rollout/internal/task"
)

// Loader loads datasets from local paths or registries.
type Loader struct {
	taskLoader *task.Loader
	resolver   *registry.Resolver
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

// LoadFromRegistry loads a dataset from a registry (local path or URL).
func (l *Loader) LoadFromRegistry(ctx context.Context, ref models.RegistryRef, name, version string) (*models.Dataset, error) {
	// Initialize resolver lazily
	if l.resolver == nil {
		r, err := registry.NewResolver()
		if err != nil {
			return nil, fmt.Errorf("creating resolver: %w", err)
		}
		l.resolver = r
		fmt.Printf("Registry clones will be stored in: %s\n", r.BaseDir())
	}

	// Load registry from path or URL
	var datasets []registry.RegistryDataset
	var err error

	if ref.Path != nil && *ref.Path != "" {
		datasets, err = registry.LoadFromPath(*ref.Path)
		if err != nil {
			return nil, fmt.Errorf("loading registry from path: %w", err)
		}
	} else if ref.URL != nil && *ref.URL != "" {
		datasets, err = registry.LoadFromURL(ctx, *ref.URL)
		if err != nil {
			return nil, fmt.Errorf("loading registry from URL: %w", err)
		}
	} else {
		return nil, fmt.Errorf("registry ref must specify either path or url")
	}

	// Find the requested dataset
	regDataset, err := registry.FindDataset(datasets, name, version)
	if err != nil {
		return nil, err
	}

	// Resolve tasks (clone repos, load tasks)
	tasks, err := l.resolver.Resolve(ctx, regDataset)
	if err != nil {
		return nil, fmt.Errorf("resolving tasks: %w", err)
	}

	return &models.Dataset{
		Name:    regDataset.Name,
		Version: regDataset.Version,
		Tasks:   tasks,
	}, nil
}
