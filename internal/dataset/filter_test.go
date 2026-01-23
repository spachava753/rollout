package dataset

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spachava753/rollout/internal/models"
)

// helper to create a task with a given name
func makeTask(name string) models.Task {
	return models.Task{Name: name}
}

// helper to create a dataset with given task names
func makeDataset(name string, taskNames ...string) *models.Dataset {
	tasks := make([]models.Task, len(taskNames))
	for i, n := range taskNames {
		tasks[i] = makeTask(n)
	}
	return &models.Dataset{
		Name:    name,
		Version: "1.0",
		Tasks:   tasks,
	}
}

// helper to extract task names from a dataset
func taskNames(ds *models.Dataset) []string {
	names := make([]string, len(ds.Tasks))
	for i, t := range ds.Tasks {
		names[i] = t.Name
	}
	return names
}

func TestFilterTasks_NilFilterReturnsUnchanged(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	result, err := FilterTasks(ds, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should return same pointer (unchanged)
	if result != ds {
		t.Error("expected same dataset pointer for nil filter")
	}
	
	// Verify tasks are unchanged
	if len(result.Tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(result.Tasks))
	}
}

func TestFilterTasks_EmptySliceReturnsError(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b")
	
	_, err := FilterTasks(ds, []string{})
	if err == nil {
		t.Fatal("expected error for empty slice filter")
	}
	
	expected := "tasks cannot be empty when specified"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestFilterTasks_ValidTaskNames(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c", "task-d")
	
	result, err := FilterTasks(ds, []string{"task-b", "task-d"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should be a new dataset
	if result == ds {
		t.Error("expected new dataset pointer")
	}
	
	// Verify correct tasks
	names := taskNames(result)
	if len(names) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(names))
	}
	if names[0] != "task-b" || names[1] != "task-d" {
		t.Errorf("expected [task-b, task-d], got %v", names)
	}
	
	// Verify metadata preserved
	if result.Name != "test-ds" {
		t.Errorf("expected name %q, got %q", "test-ds", result.Name)
	}
	if result.Version != "1.0" {
		t.Errorf("expected version %q, got %q", "1.0", result.Version)
	}
}

func TestFilterTasks_InvalidTaskNameReturnsError(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	_, err := FilterTasks(ds, []string{"task-a", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid task name")
	}
	
	// Error should be descriptive
	errStr := err.Error()
	if !strings.Contains(errStr, "nonexistent") {
		t.Errorf("error should mention missing task: %v", err)
	}
	if !strings.Contains(errStr, "test-ds") {
		t.Errorf("error should mention dataset name: %v", err)
	}
	if !strings.Contains(errStr, "available") {
		t.Errorf("error should list available tasks: %v", err)
	}
}

func TestFilterTasks_CaseSensitiveMatching(t *testing.T) {
	ds := makeDataset("test-ds", "Task-A", "task-b")
	
	// Exact case should work
	result, err := FilterTasks(ds, []string{"Task-A"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].Name != "Task-A" {
		t.Errorf("expected [Task-A], got %v", taskNames(result))
	}
	
	// Wrong case should fail
	_, err = FilterTasks(ds, []string{"task-a"})
	if err == nil {
		t.Fatal("expected error for case mismatch")
	}
	if !strings.Contains(err.Error(), "task-a") {
		t.Errorf("error should mention the missing task: %v", err)
	}
}

func TestFilterTasks_DuplicatesDeduped(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	// Filter with duplicates
	result, err := FilterTasks(ds, []string{"task-a", "task-b", "task-a", "task-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should only have 2 tasks (deduplicated)
	names := taskNames(result)
	if len(names) != 2 {
		t.Fatalf("expected 2 tasks after dedup, got %d: %v", len(names), names)
	}
	
	// Order should match first occurrence
	if names[0] != "task-a" || names[1] != "task-b" {
		t.Errorf("expected [task-a, task-b], got %v", names)
	}
}

func TestFilterTasks_OrderingMatchesFilter(t *testing.T) {
	// Dataset has tasks in order: a, b, c, d
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c", "task-d")
	
	// Filter specifies reverse order
	result, err := FilterTasks(ds, []string{"task-d", "task-b", "task-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Result should match filter order, not original order
	names := taskNames(result)
	expected := []string{"task-d", "task-b", "task-a"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d tasks, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestFilterTasks_ErrorTruncationLargeDataset(t *testing.T) {
	// Create dataset with 100+ tasks
	taskNames := make([]string, 105)
	for i := 0; i < 105; i++ {
		taskNames[i] = fmt.Sprintf("task-%03d", i)
	}
	ds := makeDataset("large-ds", taskNames...)
	
	// Request a non-existent task
	_, err := FilterTasks(ds, []string{"nonexistent-task"})
	if err == nil {
		t.Fatal("expected error")
	}
	
	errStr := err.Error()
	
	// Should contain "and N more" since we have more than 10 tasks
	if !strings.Contains(errStr, "... and 95 more") {
		t.Errorf("error should truncate to '... and 95 more': %v", err)
	}
	
	// Should show first 10 tasks
	for i := 0; i < 10; i++ {
		expected := fmt.Sprintf("task-%03d", i)
		if !strings.Contains(errStr, expected) {
			t.Errorf("error should contain first 10 tasks, missing %q: %v", expected, err)
		}
	}
}

func TestFilterTasks_AllTasksInDataset(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	// Filter with all tasks
	result, err := FilterTasks(ds, []string{"task-a", "task-b", "task-c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should still be a new dataset
	if result == ds {
		t.Error("expected new dataset pointer even when all tasks selected")
	}
	
	// Verify all tasks present
	names := taskNames(result)
	if len(names) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(names))
	}
}

func TestFilterTasks_SingleTaskDataset(t *testing.T) {
	ds := makeDataset("single-ds", "only-task")
	
	// Filter the single task
	result, err := FilterTasks(ds, []string{"only-task"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].Name != "only-task" {
		t.Errorf("expected task 'only-task', got %q", result.Tasks[0].Name)
	}
}

func TestFilterTasks_SingleTaskFilter_Exists(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	result, err := FilterTasks(ds, []string{"task-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].Name != "task-b" {
		t.Errorf("expected task 'task-b', got %q", result.Tasks[0].Name)
	}
}

func TestFilterTasks_SingleTaskFilter_DoesNotExist(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	
	_, err := FilterTasks(ds, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing task: %v", err)
	}
}

func TestFilterTasks_OriginalUnmodified(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b", "task-c")
	originalLen := len(ds.Tasks)
	
	// Filter to subset
	_, err := FilterTasks(ds, []string{"task-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Original should be unchanged
	if len(ds.Tasks) != originalLen {
		t.Errorf("original dataset modified: expected %d tasks, got %d", originalLen, len(ds.Tasks))
	}
	
	// Verify original task names unchanged
	names := taskNames(ds)
	expected := []string{"task-a", "task-b", "task-c"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("original modified at position %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestFilterTasks_MultipleNonexistentTasks(t *testing.T) {
	ds := makeDataset("test-ds", "task-a", "task-b")
	
	_, err := FilterTasks(ds, []string{"missing1", "missing2", "missing3"})
	if err == nil {
		t.Fatal("expected error")
	}
	
	errStr := err.Error()
	// All missing tasks should be mentioned
	if !strings.Contains(errStr, "missing1") {
		t.Errorf("error should mention missing1: %v", err)
	}
	if !strings.Contains(errStr, "missing2") {
		t.Errorf("error should mention missing2: %v", err)
	}
	if !strings.Contains(errStr, "missing3") {
		t.Errorf("error should mention missing3: %v", err)
	}
}

func TestFilterTasks_EmptyDataset(t *testing.T) {
	ds := &models.Dataset{
		Name:    "empty-ds",
		Version: "1.0",
		Tasks:   []models.Task{},
	}
	
	// nil filter should still work
	result, err := FilterTasks(ds, nil)
	if err != nil {
		t.Fatalf("unexpected error for nil filter on empty dataset: %v", err)
	}
	if result != ds {
		t.Error("expected same pointer for nil filter")
	}
	
	// Any filter should fail since no tasks exist
	_, err = FilterTasks(ds, []string{"any-task"})
	if err == nil {
		t.Fatal("expected error when filtering non-existent task in empty dataset")
	}
}
