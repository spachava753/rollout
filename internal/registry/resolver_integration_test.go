package registry

import (
	"context"
	"os"
	"testing"
)

// TestResolveIntegration tests the full resolve flow with real git operations.
// This test is skipped with -short flag since it requires network access.
func TestResolveIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a small test dataset with a real public repo
	dataset := &RegistryDataset{
		Name:    "test-integration",
		Version: "1.0",
		Tasks: []RegistryTask{
			{
				Name:   "hello-world",
				GitURL: "https://github.com/laude-institute/harbor.git",
				Path:   "examples/tasks/hello-world",
				// No GitCommitID means HEAD
			},
		},
	}

	resolver, err := NewResolver()
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	t.Logf("Clone directory: %s", resolver.BaseDir())

	tasks, err := resolver.Resolve(ctx, dataset)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	if len(tasks) > 0 {
		task := tasks[0]
		if task.Name != "hello-world" {
			t.Errorf("expected task name 'hello-world', got %q", task.Name)
		}
		if task.FS == nil {
			t.Error("task.FS is nil")
		}
		if task.Path == "" {
			t.Error("task.Path is empty")
		}
		t.Logf("Loaded task: name=%s, path=%s", task.Name, task.Path)
	}

	// Verify the clone directory exists
	if _, err := os.Stat(resolver.BaseDir()); os.IsNotExist(err) {
		t.Error("clone directory does not exist")
	}
}

// TestResolveWithDeduplication tests that multiple tasks from the same repo
// only result in one clone operation.
func TestResolveWithDeduplication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a dataset with multiple tasks from the same repo
	// This simulates the common case where a benchmark has many tasks from one repo
	dataset := &RegistryDataset{
		Name:    "dedup-test",
		Version: "1.0",
		Tasks: []RegistryTask{
			{
				Name:   "task-1",
				GitURL: "https://github.com/laude-institute/harbor.git",
				Path:   "examples/tasks/hello-world",
			},
			{
				Name:   "task-2", 
				GitURL: "https://github.com/laude-institute/harbor.git",
				Path:   "examples/tasks/hello-world", // Same path, different name
			},
		},
	}

	resolver, err := NewResolver()
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	t.Logf("Clone directory: %s", resolver.BaseDir())

	tasks, err := resolver.Resolve(ctx, dataset)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify tasks have different names
	names := make(map[string]bool)
	for _, task := range tasks {
		names[task.Name] = true
	}
	if len(names) != 2 {
		t.Error("tasks should have different names")
	}
}
