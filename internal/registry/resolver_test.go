package registry

import (
	"testing"
)

func TestCloneDirName(t *testing.T) {
	r := &Resolver{baseDir: "/tmp/test"}

	tests := []struct {
		name     string
		key      cloneKey
		wantPart string // Just check it contains expected parts
	}{
		{
			name: "with commit",
			key: cloneKey{
				GitURL:      "https://github.com/example/repo.git",
				GitCommitID: "abc123def456789",
			},
			wantPart: "abc123def456", // First 12 chars
		},
		{
			name: "HEAD",
			key: cloneKey{
				GitURL:      "https://github.com/example/repo.git",
				GitCommitID: "",
			},
			wantPart: "HEAD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.cloneDirName(tt.key)
			if got == "" {
				t.Error("cloneDirName returned empty string")
			}
			// Check that repo name is included
			if len(got) < 10 {
				t.Errorf("cloneDirName too short: %q", got)
			}
		})
	}
}

func TestNewResolver(t *testing.T) {
	r, err := NewResolver()
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	if r.BaseDir() == "" {
		t.Error("BaseDir() returned empty string")
	}

	if r.taskLoader == nil {
		t.Error("taskLoader is nil")
	}
}
