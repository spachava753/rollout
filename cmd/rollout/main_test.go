package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLI_TasksFlagParsing tests that the --tasks flag is parsed correctly.
// These tests build and run the CLI binary.
func TestCLI_TasksFlagParsing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "rollout")
	
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = filepath.Join(".") // cmd/rollout
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("building binary: %v\n%s", err, output)
	}

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "help flag shows usage",
			args:    []string{"-h"},
			wantErr: true, // -h causes exit 0 but no job.yaml
			errContains: "--tasks",
		},
		{
			name:        "empty tasks flag rejected",
			args:        []string{"--tasks", "", "nonexistent.yaml"},
			wantErr:     true,
			errContains: "--tasks cannot be empty",
		},
		{
			name:        "invalid task name rejected - forward slash",
			args:        []string{"--tasks", "foo/bar", "nonexistent.yaml"},
			wantErr:     true,
			errContains: "invalid task name",
		},
		{
			name:        "invalid task name rejected - starts with hyphen",
			args:        []string{"--tasks", "-task", "nonexistent.yaml"},
			wantErr:     true,
			errContains: "invalid task name",
		},
		{
			name:        "missing job.yaml shows usage",
			args:        []string{},
			wantErr:     true,
			errContains: "usage:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got none")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(outputStr), strings.ToLower(tt.errContains)) {
					t.Errorf("output %q should contain %q", outputStr, tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v\noutput: %s", err, outputStr)
			}
		})
	}
}

func TestCLI_TasksFlagCommaSeparated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "rollout")
	
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = filepath.Join(".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("building binary: %v\n%s", err, output)
	}

	// Test comma-separated parsing (will fail on nonexistent.yaml but validates parsing)
	// We just need to verify it doesn't error on the task names themselves
	cmd := exec.Command(binaryPath, "--tasks", "task-a,task-b,task-c", "nonexistent.yaml")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail on file not found, not on task validation
	if strings.Contains(outputStr, "invalid task name") {
		t.Errorf("valid comma-separated tasks should be accepted, got: %s", outputStr)
	}
}

func TestCLI_TasksFlagWhitespaceTrimming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "rollout")
	
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = filepath.Join(".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("building binary: %v\n%s", err, output)
	}

	// Test that whitespace is trimmed from task names
	cmd := exec.Command(binaryPath, "--tasks", "task-a, task-b , task-c", "nonexistent.yaml")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Should fail on file not found, not on task validation (whitespace should be trimmed)
	if strings.Contains(outputStr, "invalid task name") {
		t.Errorf("whitespace in comma-separated tasks should be trimmed, got: %s", outputStr)
	}
}
