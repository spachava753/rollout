package dataset

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spachava753/rollout/internal/models"
)

// getProjectRoot returns the absolute path to the project root.
func getProjectRoot(t *testing.T) string {
	t.Helper()
	// We're in internal/dataset, go up two levels
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// getMultiTaskDatasetPath returns the path to the multi-task test dataset.
func getMultiTaskDatasetPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(getProjectRoot(t), "testdata", "multi-task-dataset")
}

// --- Loader Tests ---

func TestLoader_LoadFromPath_Basic(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("unexpected error loading dataset: %v", err)
	}

	// Verify dataset name (should be directory name)
	if ds.Name != "multi-task-dataset" {
		t.Errorf("expected dataset name 'multi-task-dataset', got %q", ds.Name)
	}

	// Verify all 3 tasks loaded
	if len(ds.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(ds.Tasks))
	}

	// Verify task names (order may vary)
	taskNames := make(map[string]bool)
	for _, task := range ds.Tasks {
		taskNames[task.Name] = true
	}
	for _, name := range []string{"task-a", "task-b", "task-c"} {
		if !taskNames[name] {
			t.Errorf("expected task %q not found in dataset", name)
		}
	}
}

func TestLoader_LoadFromPath_NonExistent(t *testing.T) {
	loader := NewLoader()

	_, err := loader.LoadFromPath(context.Background(), "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

// --- Integration Tests: Loader + FilterTasks ---

func TestLoaderWithFilter_ValidTasks(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter to specific tasks
	filtered, err := FilterTasks(ds, []string{"task-a", "task-c"})
	if err != nil {
		t.Fatalf("filtering tasks: %v", err)
	}

	// Verify filtered result
	if len(filtered.Tasks) != 2 {
		t.Fatalf("expected 2 filtered tasks, got %d", len(filtered.Tasks))
	}

	taskNames := make([]string, len(filtered.Tasks))
	for i, task := range filtered.Tasks {
		taskNames[i] = task.Name
	}

	// Order should match filter order
	if taskNames[0] != "task-a" {
		t.Errorf("expected first task 'task-a', got %q", taskNames[0])
	}
	if taskNames[1] != "task-c" {
		t.Errorf("expected second task 'task-c', got %q", taskNames[1])
	}
}

func TestLoaderWithFilter_NilFilter_BackwardCompatible(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter with nil (should return all tasks)
	filtered, err := FilterTasks(ds, nil)
	if err != nil {
		t.Fatalf("unexpected error with nil filter: %v", err)
	}

	// Should return same dataset (unchanged)
	if filtered != ds {
		t.Error("expected same dataset pointer for nil filter")
	}

	if len(filtered.Tasks) != 3 {
		t.Errorf("expected 3 tasks (all), got %d", len(filtered.Tasks))
	}
}

func TestLoaderWithFilter_NonExistentTask_Error(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter with non-existent task
	_, err = FilterTasks(ds, []string{"task-a", "nonexistent-task"})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "nonexistent-task") {
		t.Errorf("error should mention the missing task: %v", err)
	}
	if !strings.Contains(errStr, "multi-task-dataset") {
		t.Errorf("error should mention the dataset name: %v", err)
	}
	if !strings.Contains(errStr, "available") {
		t.Errorf("error should list available tasks: %v", err)
	}
}

func TestLoaderWithFilter_AllTasksInDataset(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter with all tasks (in different order)
	filtered, err := FilterTasks(ds, []string{"task-c", "task-a", "task-b"})
	if err != nil {
		t.Fatalf("filtering with all tasks: %v", err)
	}

	if len(filtered.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(filtered.Tasks))
	}

	// Verify order matches filter order
	expectedOrder := []string{"task-c", "task-a", "task-b"}
	for i, task := range filtered.Tasks {
		if task.Name != expectedOrder[i] {
			t.Errorf("position %d: expected %q, got %q", i, expectedOrder[i], task.Name)
		}
	}
}

func TestLoaderWithFilter_SingleTask(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter to single task
	filtered, err := FilterTasks(ds, []string{"task-b"})
	if err != nil {
		t.Fatalf("filtering to single task: %v", err)
	}

	if len(filtered.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(filtered.Tasks))
	}

	if filtered.Tasks[0].Name != "task-b" {
		t.Errorf("expected task-b, got %q", filtered.Tasks[0].Name)
	}

	// Verify task properties are preserved
	if filtered.Tasks[0].Path == "" {
		t.Error("task path should be preserved")
	}
	if filtered.Tasks[0].Config.Agent.TimeoutSec != 60.0 {
		t.Errorf("task config should be preserved, got timeout=%v", filtered.Tasks[0].Config.Agent.TimeoutSec)
	}
}

func TestLoaderWithFilter_TaskPropertiesPreserved(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Get original task-a
	var originalTaskA *models.Task
	for i := range ds.Tasks {
		if ds.Tasks[i].Name == "task-a" {
			originalTaskA = &ds.Tasks[i]
			break
		}
	}
	if originalTaskA == nil {
		t.Fatal("task-a not found in original dataset")
	}

	// Filter dataset
	filtered, err := FilterTasks(ds, []string{"task-a"})
	if err != nil {
		t.Fatalf("filtering: %v", err)
	}

	// Compare filtered task with original
	filteredTask := &filtered.Tasks[0]
	if filteredTask.Name != originalTaskA.Name {
		t.Errorf("name mismatch: %q vs %q", filteredTask.Name, originalTaskA.Name)
	}
	if filteredTask.Path != originalTaskA.Path {
		t.Errorf("path mismatch: %q vs %q", filteredTask.Path, originalTaskA.Path)
	}
	if filteredTask.Config.Agent.TimeoutSec != originalTaskA.Config.Agent.TimeoutSec {
		t.Errorf("config mismatch")
	}
}

func TestLoaderWithFilter_OriginalUnmodified(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	originalTaskCount := len(ds.Tasks)
	originalTaskNames := make([]string, len(ds.Tasks))
	for i, task := range ds.Tasks {
		originalTaskNames[i] = task.Name
	}

	// Filter to subset
	_, err = FilterTasks(ds, []string{"task-b"})
	if err != nil {
		t.Fatalf("filtering: %v", err)
	}

	// Original should be unchanged
	if len(ds.Tasks) != originalTaskCount {
		t.Errorf("original modified: expected %d tasks, got %d", originalTaskCount, len(ds.Tasks))
	}

	for i, task := range ds.Tasks {
		if task.Name != originalTaskNames[i] {
			t.Errorf("original task order modified at position %d", i)
		}
	}
}

func TestLoaderWithFilter_EmptyFilter_Error(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter with empty slice should error
	_, err = FilterTasks(ds, []string{})
	if err == nil {
		t.Fatal("expected error for empty filter slice")
	}

	expected := "tasks cannot be empty when specified"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestLoaderWithFilter_CaseSensitive(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Try to filter with wrong case
	_, err = FilterTasks(ds, []string{"Task-A"})
	if err == nil {
		t.Fatal("expected error for case mismatch")
	}

	if !strings.Contains(err.Error(), "Task-A") {
		t.Errorf("error should mention the missing task name: %v", err)
	}
}

func TestLoaderWithFilter_DuplicatesInFilter(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter with duplicates
	filtered, err := FilterTasks(ds, []string{"task-a", "task-b", "task-a", "task-b"})
	if err != nil {
		t.Fatalf("filtering with duplicates: %v", err)
	}

	// Should deduplicate
	if len(filtered.Tasks) != 2 {
		t.Fatalf("expected 2 tasks after dedup, got %d", len(filtered.Tasks))
	}

	// Order should be first occurrence
	if filtered.Tasks[0].Name != "task-a" || filtered.Tasks[1].Name != "task-b" {
		t.Errorf("expected [task-a, task-b], got [%s, %s]",
			filtered.Tasks[0].Name, filtered.Tasks[1].Name)
	}
}

// --- Tests verifying task structure after loading ---

func TestLoader_TaskStructure(t *testing.T) {
	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	for _, task := range ds.Tasks {
		// Each task should have required fields populated
		if task.Name == "" {
			t.Error("task name should not be empty")
		}
		if task.Path == "" {
			t.Errorf("task %s: path should not be empty", task.Name)
		}
		if task.FS == nil {
			t.Errorf("task %s: FS should not be nil", task.Name)
		}

		// Verify config loaded
		if task.Config.Agent.TimeoutSec == 0 {
			t.Errorf("task %s: agent timeout should be set", task.Name)
		}
		if task.Config.Verifier.TimeoutSec == 0 {
			t.Errorf("task %s: verifier timeout should be set", task.Name)
		}
	}
}

// --- End-to-end simulation of what orchestrator does ---

func TestLoaderFilterWorkflow_SimulateOrchestrator(t *testing.T) {
	// This test simulates what the orchestrator does:
	// 1. Load dataset
	// 2. Apply filter if tasks specified
	// 3. Iterate over filtered tasks

	loader := NewLoader()
	datasetPath := getMultiTaskDatasetPath(t)

	// Simulate DatasetRef with tasks filter
	type DatasetRef struct {
		Path  string
		Tasks []string
	}

	testCases := []struct {
		name          string
		ref           DatasetRef
		expectedTasks []string
		expectError   bool
	}{
		{
			name: "no filter - all tasks",
			ref: DatasetRef{
				Path:  datasetPath,
				Tasks: nil,
			},
			expectedTasks: []string{"task-a", "task-b", "task-c"},
			expectError:   false,
		},
		{
			name: "filter to subset",
			ref: DatasetRef{
				Path:  datasetPath,
				Tasks: []string{"task-c", "task-a"},
			},
			expectedTasks: []string{"task-c", "task-a"},
			expectError:   false,
		},
		{
			name: "filter with nonexistent task",
			ref: DatasetRef{
				Path:  datasetPath,
				Tasks: []string{"task-a", "nonexistent"},
			},
			expectedTasks: nil,
			expectError:   true,
		},
		{
			name: "single task filter",
			ref: DatasetRef{
				Path:  datasetPath,
				Tasks: []string{"task-b"},
			},
			expectedTasks: []string{"task-b"},
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Load dataset
			ds, err := loader.LoadFromPath(context.Background(), tc.ref.Path)
			if err != nil {
				t.Fatalf("loading dataset: %v", err)
			}

			// Step 2: Apply filter if specified
			if tc.ref.Tasks != nil {
				ds, err = FilterTasks(ds, tc.ref.Tasks)
				if tc.expectError {
					if err == nil {
						t.Fatal("expected error, got nil")
					}
					return // Error case passed
				}
				if err != nil {
					t.Fatalf("unexpected filter error: %v", err)
				}
			}

			// Step 3: Verify tasks
			if tc.expectError {
				t.Fatal("expected error but workflow succeeded")
			}

			if len(ds.Tasks) != len(tc.expectedTasks) {
				t.Fatalf("expected %d tasks, got %d", len(tc.expectedTasks), len(ds.Tasks))
			}

			// For nil filter, just check all tasks present (order may vary)
			if tc.ref.Tasks == nil {
				taskMap := make(map[string]bool)
				for _, task := range ds.Tasks {
					taskMap[task.Name] = true
				}
				for _, expected := range tc.expectedTasks {
					if !taskMap[expected] {
						t.Errorf("expected task %q not found", expected)
					}
				}
			} else {
				// For explicit filter, check exact order
				for i, task := range ds.Tasks {
					if task.Name != tc.expectedTasks[i] {
						t.Errorf("position %d: expected %q, got %q", i, tc.expectedTasks[i], task.Name)
					}
				}
			}
		})
	}
}

// --- Single-task dataset test ---

// createSingleTaskDataset creates a temp dataset with one task for testing edge cases.
func createSingleTaskDataset(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()
	taskDir := filepath.Join(baseDir, "only-task")
	envDir := filepath.Join(taskDir, "environment")
	testsDir := filepath.Join(taskDir, "tests")

	if err := os.MkdirAll(envDir, 0755); err != nil {
		t.Fatalf("creating env dir: %v", err)
	}
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatalf("creating tests dir: %v", err)
	}

	// Write minimal task files
	taskToml := `version = "1.0"
[verifier]
timeout_sec = 60.0
[agent]
timeout_sec = 60.0
`
	if err := os.WriteFile(filepath.Join(taskDir, "task.toml"), []byte(taskToml), 0644); err != nil {
		t.Fatalf("writing task.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "instruction.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("writing instruction.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "Dockerfile"), []byte("FROM alpine:latest\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "test.sh"), []byte("#!/bin/bash\nexit 0\n"), 0755); err != nil {
		t.Fatalf("writing test.sh: %v", err)
	}

	return baseDir
}

func TestLoaderWithFilter_SingleTaskDataset_FilterSame(t *testing.T) {
	loader := NewLoader()
	datasetPath := createSingleTaskDataset(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	if len(ds.Tasks) != 1 {
		t.Fatalf("expected 1 task in single-task dataset, got %d", len(ds.Tasks))
	}

	// Filter to the same single task
	filtered, err := FilterTasks(ds, []string{"only-task"})
	if err != nil {
		t.Fatalf("filtering single task dataset: %v", err)
	}

	if len(filtered.Tasks) != 1 {
		t.Fatalf("expected 1 task after filter, got %d", len(filtered.Tasks))
	}

	if filtered.Tasks[0].Name != "only-task" {
		t.Errorf("expected 'only-task', got %q", filtered.Tasks[0].Name)
	}
}

func TestLoaderWithFilter_SingleTaskDataset_FilterNonexistent(t *testing.T) {
	loader := NewLoader()
	datasetPath := createSingleTaskDataset(t)

	// Load dataset
	ds, err := loader.LoadFromPath(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("loading dataset: %v", err)
	}

	// Filter to nonexistent task
	_, err = FilterTasks(ds, []string{"different-task"})
	if err == nil {
		t.Fatal("expected error for nonexistent task in single-task dataset")
	}

	if !strings.Contains(err.Error(), "different-task") {
		t.Errorf("error should mention missing task: %v", err)
	}
}
