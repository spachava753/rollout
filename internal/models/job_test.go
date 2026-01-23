package models

import (
	"encoding/json"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDatasetRef_TasksField_YAMLUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		yamlStr  string
		wantNil  bool
		wantLen  int
		wantVals []string
	}{
		{
			name: "tasks with values",
			yamlStr: `
path: ./test
tasks:
  - a
  - b
`,
			wantNil:  false,
			wantLen:  2,
			wantVals: []string{"a", "b"},
		},
		{
			name: "tasks inline syntax",
			yamlStr: `
path: ./test
tasks: [task-1, task-2, task-3]
`,
			wantNil:  false,
			wantLen:  3,
			wantVals: []string{"task-1", "task-2", "task-3"},
		},
		{
			name: "empty tasks array",
			yamlStr: `
path: ./test
tasks: []
`,
			wantNil:  false,
			wantLen:  0,
			wantVals: []string{},
		},
		{
			name: "tasks field omitted",
			yamlStr: `
path: ./test
`,
			wantNil:  true,
			wantLen:  0,
			wantVals: nil,
		},
		{
			name: "single task",
			yamlStr: `
path: ./test
tasks:
  - only-one
`,
			wantNil:  false,
			wantLen:  1,
			wantVals: []string{"only-one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ref DatasetRef
			if err := yaml.Unmarshal([]byte(tt.yamlStr), &ref); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}

			if tt.wantNil {
				if ref.Tasks != nil {
					t.Errorf("Tasks = %v, want nil", ref.Tasks)
				}
			} else {
				if ref.Tasks == nil {
					t.Fatalf("Tasks = nil, want non-nil slice")
				}
				if len(ref.Tasks) != tt.wantLen {
					t.Errorf("len(Tasks) = %d, want %d", len(ref.Tasks), tt.wantLen)
				}
				if !reflect.DeepEqual(ref.Tasks, tt.wantVals) {
					t.Errorf("Tasks = %v, want %v", ref.Tasks, tt.wantVals)
				}
			}
		})
	}
}

func TestDatasetRef_TasksField_EmptyVsNil(t *testing.T) {
	// Verify that empty slice and nil are distinguishable after YAML unmarshal
	yamlEmpty := `tasks: []`
	yamlOmitted := `name: test`

	var refEmpty DatasetRef
	if err := yaml.Unmarshal([]byte(yamlEmpty), &refEmpty); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}

	var refOmitted DatasetRef
	if err := yaml.Unmarshal([]byte(yamlOmitted), &refOmitted); err != nil {
		t.Fatalf("unmarshal omitted: %v", err)
	}

	// Empty slice should be non-nil with length 0
	if refEmpty.Tasks == nil {
		t.Error("empty tasks should be non-nil slice, got nil")
	}
	if len(refEmpty.Tasks) != 0 {
		t.Errorf("empty tasks should have length 0, got %d", len(refEmpty.Tasks))
	}

	// Omitted should be nil
	if refOmitted.Tasks != nil {
		t.Errorf("omitted tasks should be nil, got %v", refOmitted.Tasks)
	}
}

func TestDatasetRef_TasksField_JSONMarshal(t *testing.T) {
	tests := []struct {
		name     string
		ref      DatasetRef
		contains string
		excludes string
	}{
		{
			name: "with tasks",
			ref: DatasetRef{
				Name:  "test-dataset",
				Tasks: []string{"task-a", "task-b"},
			},
			contains: `"tasks":["task-a","task-b"]`,
		},
		{
			name: "empty tasks omitted with omitempty",
			ref: DatasetRef{
				Name:  "test-dataset",
				Tasks: []string{},
			},
			// Note: omitempty causes empty slices to be omitted in JSON
			excludes: `"tasks"`,
		},
		{
			name: "nil tasks omitted",
			ref: DatasetRef{
				Name:  "test-dataset",
				Tasks: nil,
			},
			excludes: `"tasks"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.ref)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			jsonStr := string(data)
			if tt.contains != "" {
				found := false
				if len(jsonStr) > 0 {
					found = containsSubstring(jsonStr, tt.contains)
				}
				if !found {
					t.Errorf("JSON = %s, want to contain %s", jsonStr, tt.contains)
				}
			}
			if tt.excludes != "" {
				if containsSubstring(jsonStr, tt.excludes) {
					t.Errorf("JSON = %s, should not contain %s", jsonStr, tt.excludes)
				}
			}
		})
	}
}

func TestDatasetRef_TasksField_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantNil  bool
		wantVals []string
	}{
		{
			name:     "with tasks",
			jsonStr:  `{"name":"test","tasks":["a","b"]}`,
			wantNil:  false,
			wantVals: []string{"a", "b"},
		},
		{
			name:     "empty tasks",
			jsonStr:  `{"name":"test","tasks":[]}`,
			wantNil:  false,
			wantVals: []string{},
		},
		{
			name:     "tasks omitted",
			jsonStr:  `{"name":"test"}`,
			wantNil:  true,
			wantVals: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ref DatasetRef
			if err := json.Unmarshal([]byte(tt.jsonStr), &ref); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if tt.wantNil {
				if ref.Tasks != nil {
					t.Errorf("Tasks = %v, want nil", ref.Tasks)
				}
			} else {
				if ref.Tasks == nil {
					t.Fatalf("Tasks = nil, want non-nil slice")
				}
				if !reflect.DeepEqual(ref.Tasks, tt.wantVals) {
					t.Errorf("Tasks = %v, want %v", ref.Tasks, tt.wantVals)
				}
			}
		})
	}
}

func TestDatasetRef_TasksField_YAMLMarshal(t *testing.T) {
	tests := []struct {
		name     string
		ref      DatasetRef
		contains string
		excludes string
	}{
		{
			name: "with tasks",
			ref: DatasetRef{
				Name:  "test-dataset",
				Tasks: []string{"task-a", "task-b"},
			},
			contains: "tasks:",
		},
		{
			name: "nil tasks omitted",
			ref: DatasetRef{
				Name:  "test-dataset",
				Tasks: nil,
			},
			excludes: "tasks:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.ref)
			if err != nil {
				t.Fatalf("yaml.Marshal() error = %v", err)
			}

			yamlStr := string(data)
			if tt.contains != "" {
				if !containsSubstring(yamlStr, tt.contains) {
					t.Errorf("YAML = %s, want to contain %s", yamlStr, tt.contains)
				}
			}
			if tt.excludes != "" {
				if containsSubstring(yamlStr, tt.excludes) {
					t.Errorf("YAML = %s, should not contain %s", yamlStr, tt.excludes)
				}
			}
		})
	}
}

func TestDatasetRef_TasksField_RoundTrip(t *testing.T) {
	original := DatasetRef{
		Name:    "my-dataset",
		Version: "1.0",
		Tasks:   []string{"task-1", "task-2", "task-3"},
	}

	// YAML round trip
	yamlData, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	var fromYAML DatasetRef
	if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(original.Tasks, fromYAML.Tasks) {
		t.Errorf("YAML round trip: got Tasks = %v, want %v", fromYAML.Tasks, original.Tasks)
	}

	// JSON round trip
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var fromJSON DatasetRef
	if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(original.Tasks, fromJSON.Tasks) {
		t.Errorf("JSON round trip: got Tasks = %v, want %v", fromJSON.Tasks, original.Tasks)
	}
}

// containsSubstring checks if s contains substr (helper for tests)
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
