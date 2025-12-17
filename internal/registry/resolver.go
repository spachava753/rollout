package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/spachava753/rollout/internal/models"
	"github.com/spachava753/rollout/internal/task"
)

// Resolver resolves registry tasks by cloning git repositories and loading tasks.
type Resolver struct {
	taskLoader *task.Loader
	baseDir    string // Base directory for clones
}

// NewResolver creates a new Resolver.
// The baseDir will be created under os.TempDir() with a timestamp prefix.
func NewResolver() (*Resolver, error) {
	baseDir := filepath.Join(os.TempDir(), fmt.Sprintf("rollout-registry-%d", time.Now().Unix()))
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating base directory: %w", err)
	}

	return &Resolver{
		taskLoader: task.NewLoader(),
		baseDir:    baseDir,
	}, nil
}

// BaseDir returns the base directory where repositories are cloned.
func (r *Resolver) BaseDir() string {
	return r.baseDir
}

// Resolve resolves all tasks in a registry dataset by cloning the necessary
// repositories and loading each task. Repositories are deduplicated by
// (git_url, git_commit_id) to avoid redundant clones.
func (r *Resolver) Resolve(ctx context.Context, dataset *RegistryDataset) ([]models.Task, error) {
	// Group tasks by clone key for deduplication
	groups := make(map[cloneKey][]RegistryTask)
	for _, t := range dataset.Tasks {
		key := cloneKey{GitURL: t.GitURL, GitCommitID: t.GitCommitID}
		groups[key] = append(groups[key], t)
	}

	// Clone each unique repository (parallel)
	clones := make(map[cloneKey]string)
	var clonesMu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	for key := range groups {
		g.Go(func() error {
			clonePath, err := r.cloneRepo(ctx, key)
			if err != nil {
				return fmt.Errorf("cloning %s: %w", key.GitURL, err)
			}
			clonesMu.Lock()
			clones[key] = clonePath
			clonesMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Load tasks from cloned repositories
	var tasks []models.Task
	for _, regTask := range dataset.Tasks {
		key := cloneKey{GitURL: regTask.GitURL, GitCommitID: regTask.GitCommitID}
		clonePath := clones[key]

		taskPath := clonePath
		if regTask.Path != "" {
			taskPath = filepath.Join(clonePath, regTask.Path)
		}

		t, err := r.taskLoader.LoadTask(ctx, taskPath)
		if err != nil {
			return nil, fmt.Errorf("loading task %s: %w", regTask.Name, err)
		}

		if err := r.taskLoader.ValidateTask(t); err != nil {
			return nil, fmt.Errorf("validating task %s: %w", regTask.Name, err)
		}

		// Override task name with registry name and set git commit ID
		t.Name = regTask.Name
		if regTask.GitCommitID != "" {
			t.GitCommitID = &regTask.GitCommitID
		}

		tasks = append(tasks, *t)
	}

	return tasks, nil
}

// cloneRepo clones a repository to baseDir. For specific commits, it does a full
// clone then checks out the commit. For HEAD, it does a shallow clone.
func (r *Resolver) cloneRepo(ctx context.Context, key cloneKey) (string, error) {
	// Create a unique directory name based on URL and commit
	dirName := r.cloneDirName(key)
	clonePath := filepath.Join(r.baseDir, dirName)

	// Check if already cloned (idempotent)
	if _, err := os.Stat(clonePath); err == nil {
		return clonePath, nil
	}

	if key.GitCommitID == "" {
		// Shallow clone for HEAD
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", key.GitURL, clonePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}
	} else {
		// Full clone then checkout specific commit
		cmd := exec.CommandContext(ctx, "git", "clone", key.GitURL, clonePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}

		cmd = exec.CommandContext(ctx, "git", "checkout", key.GitCommitID)
		cmd.Dir = clonePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git checkout %s: %w", key.GitCommitID, err)
		}
	}

	return clonePath, nil
}

// cloneDirName generates a unique directory name for a clone key.
func (r *Resolver) cloneDirName(key cloneKey) string {
	// Hash the URL to get a short, filesystem-safe name
	h := sha256.Sum256([]byte(key.GitURL))
	urlHash := fmt.Sprintf("%x", h[:8])

	commitPart := "HEAD"
	if key.GitCommitID != "" {
		// Use first 12 chars of commit ID
		commitPart = key.GitCommitID
		if len(commitPart) > 12 {
			commitPart = commitPart[:12]
		}
	}

	// Extract repo name from URL for readability
	repoName := filepath.Base(strings.TrimSuffix(key.GitURL, ".git"))

	return fmt.Sprintf("%s-%s-%s", repoName, urlHash, commitPart)
}
