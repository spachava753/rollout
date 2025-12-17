package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPath(t *testing.T) {
	// Create a temporary registry file
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	datasets := []RegistryDataset{
		{
			Name:        "test-dataset",
			Version:     "1.0",
			Description: "A test dataset",
			Tasks: []RegistryTask{
				{
					Name:        "task-1",
					GitURL:      "https://github.com/example/repo.git",
					GitCommitID: "abc123",
					Path:        "tasks/task-1",
				},
			},
		},
	}

	data, err := json.Marshal(datasets)
	if err != nil {
		t.Fatalf("marshaling test data: %v", err)
	}

	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		t.Fatalf("writing test registry: %v", err)
	}

	// Test loading
	loaded, err := LoadFromPath(registryPath)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("expected 1 dataset, got %d", len(loaded))
	}

	if loaded[0].Name != "test-dataset" {
		t.Errorf("expected name 'test-dataset', got %q", loaded[0].Name)
	}

	if len(loaded[0].Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(loaded[0].Tasks))
	}
}

func TestLoadFromPath_NotFound(t *testing.T) {
	_, err := LoadFromPath("/nonexistent/path/registry.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFromPath_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	if err := os.WriteFile(registryPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := LoadFromPath(registryPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadFromURL(t *testing.T) {
	datasets := []RegistryDataset{
		{
			Name:    "url-dataset",
			Version: "2.0",
			Tasks: []RegistryTask{
				{
					Name:   "url-task",
					GitURL: "https://github.com/example/repo.git",
				},
			},
		},
	}

	data, err := json.Marshal(datasets)
	if err != nil {
		t.Fatalf("marshaling test data: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer server.Close()

	ctx := context.Background()
	loaded, err := LoadFromURL(ctx, server.URL)
	if err != nil {
		t.Fatalf("LoadFromURL: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("expected 1 dataset, got %d", len(loaded))
	}

	if loaded[0].Name != "url-dataset" {
		t.Errorf("expected name 'url-dataset', got %q", loaded[0].Name)
	}
}

func TestLoadFromURL_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := LoadFromURL(ctx, server.URL)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestFindDataset(t *testing.T) {
	datasets := []RegistryDataset{
		{Name: "ds-1", Version: "1.0"},
		{Name: "ds-1", Version: "2.0"},
		{Name: "ds-2", Version: "1.0"},
	}

	tests := []struct {
		name        string
		dsName      string
		version     string
		wantName    string
		wantVersion string
		wantErr     bool
	}{
		{"exact match", "ds-1", "2.0", "ds-1", "2.0", false},
		{"first match no version", "ds-1", "", "ds-1", "1.0", false},
		{"not found", "ds-3", "", "", "", true},
		{"version not found", "ds-1", "3.0", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindDataset(datasets, tt.dsName, tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindDataset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if got.Name != tt.wantName {
					t.Errorf("FindDataset() name = %v, want %v", got.Name, tt.wantName)
				}
				if got.Version != tt.wantVersion {
					t.Errorf("FindDataset() version = %v, want %v", got.Version, tt.wantVersion)
				}
			}
		})
	}
}
