package task

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spachava753/rollout/internal/config"
	"github.com/spachava753/rollout/internal/models"
)

// Loader loads tasks from various sources.
type Loader struct{}

// NewLoader creates a new task loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadTask loads a single task from a filesystem path.
func (l *Loader) LoadTask(ctx context.Context, taskPath string) (*models.Task, error) {
	// Get absolute path for git operations
	absPath, err := filepath.Abs(taskPath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	// Create fs.FS from directory
	fsys := os.DirFS(taskPath)

	// Load task config
	cfg, err := config.LoadTaskConfig(fsys)
	if err != nil {
		return nil, fmt.Errorf("loading task config: %w", err)
	}

	// Get task name from directory
	name := filepath.Base(absPath)

	// Try to resolve git commit ID
	var gitCommitID *string
	if sha := resolveGitSHA(absPath); sha != "" {
		gitCommitID = &sha
	}

	task := &models.Task{
		Name:        name,
		Path:        absPath,
		FS:          fsys,
		Config:      cfg,
		GitCommitID: gitCommitID,
	}

	return task, nil
}

// ValidateTask validates a task's structure and configuration.
func (l *Loader) ValidateTask(task *models.Task) error {
	// Check instruction.md exists
	if _, err := fs.Stat(task.FS, "instruction.md"); err != nil {
		return fmt.Errorf("instruction.md not found: %w", err)
	}

	// Check environment directory exists
	if _, err := fs.Stat(task.FS, "environment"); err != nil {
		return fmt.Errorf("environment directory not found: %w", err)
	}

	// Check tests directory and test.sh exist
	if _, err := fs.Stat(task.FS, "tests/test.sh"); err != nil {
		return fmt.Errorf("tests/test.sh not found: %w", err)
	}

	return nil
}

// resolveGitSHA attempts to get the current HEAD commit SHA.
func resolveGitSHA(path string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
