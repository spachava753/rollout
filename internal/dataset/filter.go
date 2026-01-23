package dataset

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spachava753/rollout/internal/models"
)

// FilterTasks filters a dataset's tasks based on the provided task names.
// Returns a NEW Dataset with only the specified tasks; does not modify the original.
// If taskNames is nil, returns the dataset unchanged.
// If taskNames is empty slice, returns error ("tasks cannot be empty when specified").
// Returns error if any specified task is not found in the dataset.
// Matching is case-sensitive (task names come from directory names).
// Duplicate task names in the filter are silently deduplicated (first occurrence preserved).
func FilterTasks(ds *models.Dataset, taskNames []string) (*models.Dataset, error) {
	// Check for nil dataset to prevent panic
	if ds == nil {
		return nil, fmt.Errorf("dataset cannot be nil")
	}

	// nil filter means return all tasks unchanged
	if taskNames == nil {
		return ds, nil
	}

	// Empty slice is an error
	if len(taskNames) == 0 {
		return nil, fmt.Errorf("tasks cannot be empty when specified")
	}

	// Build a map of available tasks for O(1) lookup
	taskMap := make(map[string]models.Task, len(ds.Tasks))
	for _, t := range ds.Tasks {
		taskMap[t.Name] = t
	}

	// Deduplicate taskNames while preserving first occurrence order
	seen := make(map[string]bool, len(taskNames))
	uniqueNames := make([]string, 0, len(taskNames))
	for _, name := range taskNames {
		if seen[name] {
			slog.Debug("duplicate task name in filter, using first occurrence", "name", name)
			continue
		}
		seen[name] = true
		uniqueNames = append(uniqueNames, name)
	}

	// Find missing tasks
	var missingTasks []string
	for _, name := range uniqueNames {
		if _, exists := taskMap[name]; !exists {
			missingTasks = append(missingTasks, name)
		}
	}

	// Report errors for missing tasks
	if len(missingTasks) > 0 {
		// Build available tasks list for error message (limit to first 10)
		availableNames := make([]string, 0, len(ds.Tasks))
		for _, t := range ds.Tasks {
			availableNames = append(availableNames, t.Name)
		}

		availableStr := formatTaskList(availableNames, 10)
		missingStr := formatMissingTasks(missingTasks)

		return nil, fmt.Errorf("tasks not found in dataset %q: [%s] (available: %s)",
			ds.Name, missingStr, availableStr)
	}

	// Build filtered task list in filter order
	filteredTasks := make([]models.Task, 0, len(uniqueNames))
	for _, name := range uniqueNames {
		filteredTasks = append(filteredTasks, taskMap[name])
	}

	// Return new Dataset (does not modify original)
	return &models.Dataset{
		Name:    ds.Name,
		Version: ds.Version,
		Tasks:   filteredTasks,
	}, nil
}

// formatTaskList formats a list of task names, truncating if more than maxShow.
func formatTaskList(names []string, maxShow int) string {
	if len(names) <= maxShow {
		return strings.Join(names, ", ")
	}
	truncated := names[:maxShow]
	remaining := len(names) - maxShow
	return fmt.Sprintf("%s, ... and %d more", strings.Join(truncated, ", "), remaining)
}

// formatMissingTasks formats missing task names for error messages.
func formatMissingTasks(names []string) string {
	return strings.Join(names, ", ")
}
