package task_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spachava753/rollout/internal/task"
)

func TestLoadTask(t *testing.T) {
	// Use the actual test-dataset/hello-world task
	projectRoot := findProjectRoot(t)
	taskPath := filepath.Join(projectRoot, "test-dataset", "hello-world")

	loader := task.NewLoader()
	loadedTask, err := loader.LoadTask(context.Background(), taskPath)
	if err != nil {
		t.Fatalf("LoadTask failed: %v", err)
	}

	if loadedTask.Name != "hello-world" {
		t.Errorf("expected task name hello-world, got %s", loadedTask.Name)
	}

	if loadedTask.Config.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", loadedTask.Config.Version)
	}

	if loadedTask.Config.Verifier.TimeoutSec != 120.0 {
		t.Errorf("expected verifier timeout 120, got %f", loadedTask.Config.Verifier.TimeoutSec)
	}
}

func TestValidateTask(t *testing.T) {
	projectRoot := findProjectRoot(t)
	taskPath := filepath.Join(projectRoot, "test-dataset", "hello-world")

	loader := task.NewLoader()
	loadedTask, err := loader.LoadTask(context.Background(), taskPath)
	if err != nil {
		t.Fatalf("LoadTask failed: %v", err)
	}

	if err := loader.ValidateTask(loadedTask); err != nil {
		t.Errorf("ValidateTask failed: %v", err)
	}
}

func TestTaskAccessors(t *testing.T) {
	projectRoot := findProjectRoot(t)
	taskPath := filepath.Join(projectRoot, "test-dataset", "hello-world")

	loader := task.NewLoader()
	loadedTask, err := loader.LoadTask(context.Background(), taskPath)
	if err != nil {
		t.Fatalf("LoadTask failed: %v", err)
	}

	// Test Instruction()
	instrFile, err := loadedTask.Instruction()
	if err != nil {
		t.Errorf("Instruction() failed: %v", err)
	}
	instrFile.Close()

	// Test Environment()
	envFS, err := loadedTask.Environment()
	if err != nil {
		t.Errorf("Environment() failed: %v", err)
	}
	if _, err := envFS.Open("Dockerfile"); err != nil {
		t.Errorf("Dockerfile not found in environment: %v", err)
	}

	// Test Solution()
	solFS, err := loadedTask.Solution()
	if err != nil {
		t.Errorf("Solution() failed: %v", err)
	}
	if _, err := solFS.Open("solve.sh"); err != nil {
		t.Errorf("solve.sh not found in solution: %v", err)
	}

	// Test Tests()
	testsFS, err := loadedTask.Tests()
	if err != nil {
		t.Errorf("Tests() failed: %v", err)
	}
	if _, err := testsFS.Open("test.sh"); err != nil {
		t.Errorf("test.sh not found in tests: %v", err)
	}
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	// Start from current dir and walk up to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working dir: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}
