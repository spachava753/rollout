package apple

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/spachava753/rollout/internal/environment"
)

func TestParseProviderConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   ProviderConfig
	}{
		{
			name:   "nil config",
			config: nil,
			want:   ProviderConfig{},
		},
		{
			name:   "empty config",
			config: map[string]any{},
			want:   ProviderConfig{},
		},
		{
			name: "with runtime_user",
			config: map[string]any{
				"runtime_user": "1001",
			},
			want: ProviderConfig{RuntimeUser: "1001"},
		},
		{
			name: "with runtime_user and runtime_group",
			config: map[string]any{
				"runtime_user":  "1001",
				"runtime_group": "1002",
			},
			want: ProviderConfig{RuntimeUser: "1001", RuntimeGroup: "1002"},
		},
		{
			name: "with invalid types (ignored)",
			config: map[string]any{
				"runtime_user": 1001, // int instead of string
			},
			want: ProviderConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseProviderConfig(tt.config)
			if got != tt.want {
				t.Errorf("ParseProviderConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "absolute path",
			path:    "/app/data",
			wantErr: false,
		},
		{
			name:    "relative path",
			path:    "./relative",
			wantErr: false,
		},
		{
			name:    "nested path",
			path:    "/app/foo/bar/baz",
			wantErr: false,
		},
		{
			name:    "parent traversal",
			path:    "../escape",
			wantErr: true,
		},
		{
			name:    "nested parent traversal",
			path:    "/app/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "double dot in name (not traversal)",
			path:    "/app/file..txt",
			wantErr: false,
		},
		{
			name:    "hidden traversal",
			path:    "/app/foo/../../etc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestRunPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("successful pipe", func(t *testing.T) {
		cmd1 := exec.CommandContext(ctx, "echo", "hello")
		cmd2 := exec.CommandContext(ctx, "cat")
		var out bytes.Buffer
		cmd2.Stdout = &out

		err := runPipeline(ctx, cmd1, cmd2)
		if err != nil {
			t.Errorf("runPipeline() error = %v", err)
		}
		if got := out.String(); got != "hello\n" {
			t.Errorf("runPipeline() output = %q, want %q", got, "hello\n")
		}
	})

	t.Run("first command fails", func(t *testing.T) {
		cmd1 := exec.CommandContext(ctx, "false") // exits with 1
		cmd2 := exec.CommandContext(ctx, "cat")

		err := runPipeline(ctx, cmd1, cmd2)
		if err == nil {
			t.Error("runPipeline() expected error when first command fails")
		}
	})

	t.Run("second command fails", func(t *testing.T) {
		cmd1 := exec.CommandContext(ctx, "echo", "hello")
		cmd2 := exec.CommandContext(ctx, "false")

		err := runPipeline(ctx, cmd1, cmd2)
		if err == nil {
			t.Error("runPipeline() expected error when second command fails")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		cmd1 := exec.CommandContext(ctx, "sleep", "10")
		cmd2 := exec.CommandContext(ctx, "cat")

		err := runPipeline(ctx, cmd1, cmd2)
		if err == nil {
			t.Error("runPipeline() expected error on context cancellation")
		}
	})
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"1000", true},
		{"0", true},
		{"", false},
		{"abc", false},
		{"1a2", false},
		{"-1", false},
	}

	for _, tt := range tests {
		if got := isNumeric(tt.s); got != tt.want {
			t.Errorf("isNumeric(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

// containerAvailable checks if Apple Container CLI is available.
func containerAvailable() bool {
	_, err := exec.LookPath("container")
	return err == nil
}

func TestNewProvider(t *testing.T) {
	if !containerAvailable() {
		t.Skip("container CLI not available")
	}

	provider, err := NewProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if provider.Name() != "apple" {
		t.Errorf("Provider.Name() = %q, want %q", provider.Name(), "apple")
	}
}

func TestIntegration(t *testing.T) {
	if !containerAvailable() {
		t.Skip("container CLI not available")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	provider, err := NewProvider(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	// Pull a simple image
	t.Run("PullImage", func(t *testing.T) {
		if err := provider.PullImage(ctx, "alpine:latest"); err != nil {
			t.Fatalf("PullImage() error = %v", err)
		}
	})

	// Create environment
	var env *Environment
	t.Run("CreateEnvironment", func(t *testing.T) {
		e, err := provider.CreateEnvironment(ctx, environment.CreateEnvironmentOptions{
			Name:     "rollout-test-" + time.Now().Format("20060102150405"),
			ImageRef: "alpine:latest",
		})
		if err != nil {
			t.Fatalf("CreateEnvironment() error = %v", err)
		}
		env = e.(*Environment)
		t.Logf("Created container: %s (uid=%s, gid=%s)", env.ID(), env.runtimeUID, env.runtimeGID)
	})

	// Clean up at end
	defer func() {
		if env != nil {
			env.Destroy(ctx)
		}
	}()

	// Test Exec
	t.Run("Exec", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode, err := env.Exec(ctx, "echo hello", &stdout, &stderr, environment.ExecOptions{})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if exitCode != 0 {
			t.Errorf("Exec() exitCode = %d, want 0", exitCode)
		}
		if got := stdout.String(); got != "hello\n" {
			t.Errorf("Exec() stdout = %q, want %q", got, "hello\n")
		}
	})

	t.Run("Exec with env vars", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode, err := env.Exec(ctx, "echo $MY_VAR", &stdout, &stderr, environment.ExecOptions{
			Env: map[string]string{"MY_VAR": "test_value"},
		})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if exitCode != 0 {
			t.Errorf("Exec() exitCode = %d, want 0", exitCode)
		}
		if got := stdout.String(); got != "test_value\n" {
			t.Errorf("Exec() stdout = %q, want %q", got, "test_value\n")
		}
	})

	t.Run("Exec non-zero exit", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode, err := env.Exec(ctx, "exit 42", &stdout, &stderr, environment.ExecOptions{})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if exitCode != 42 {
			t.Errorf("Exec() exitCode = %d, want 42", exitCode)
		}
	})

	// Test CopyTo and CopyFrom
	t.Run("CopyTo file", func(t *testing.T) {
		// Create temp file
		tmpFile, err := os.CreateTemp("", "rollout-test-*.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString("test content"); err != nil {
			t.Fatal(err)
		}
		tmpFile.Close()

		// Copy to container
		if err := env.CopyTo(ctx, tmpFile.Name(), "/tmp/test.txt"); err != nil {
			t.Fatalf("CopyTo() error = %v", err)
		}

		// Verify content
		var stdout, stderr bytes.Buffer
		_, err = env.Exec(ctx, "cat /tmp/test.txt", &stdout, &stderr, environment.ExecOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != "test content" {
			t.Errorf("CopyTo() content = %q, want %q", got, "test content")
		}
	})

	t.Run("CopyTo directory", func(t *testing.T) {
		// Create temp directory with files
		tmpDir, err := os.MkdirTemp("", "rollout-test-dir-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)

		// Copy to container
		if err := env.CopyTo(ctx, tmpDir, "/tmp/testdir"); err != nil {
			t.Fatalf("CopyTo() error = %v", err)
		}

		// Verify content
		var stdout bytes.Buffer
		env.Exec(ctx, "cat /tmp/testdir/file1.txt", &stdout, nil, environment.ExecOptions{})
		if got := stdout.String(); got != "content1" {
			t.Errorf("CopyTo() file1 content = %q, want %q", got, "content1")
		}
	})

	t.Run("CopyFrom file", func(t *testing.T) {
		// Create file in container
		env.Exec(ctx, "echo -n 'from container' > /tmp/from.txt", nil, nil, environment.ExecOptions{})

		// Copy from container
		tmpDir, _ := os.MkdirTemp("", "rollout-from-*")
		defer os.RemoveAll(tmpDir)

		dstFile := filepath.Join(tmpDir, "from.txt")
		if err := env.CopyFrom(ctx, "/tmp/from.txt", dstFile); err != nil {
			t.Fatalf("CopyFrom() error = %v", err)
		}

		content, err := os.ReadFile(dstFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "from container" {
			t.Errorf("CopyFrom() content = %q, want %q", string(content), "from container")
		}
	})

	t.Run("CopyFrom directory", func(t *testing.T) {
		// Create directory in container
		env.Exec(ctx, "mkdir -p /tmp/fromdir && echo -n 'a' > /tmp/fromdir/a.txt && echo -n 'b' > /tmp/fromdir/b.txt", nil, nil, environment.ExecOptions{})

		// Copy from container
		tmpDir, _ := os.MkdirTemp("", "rollout-fromdir-*")
		defer os.RemoveAll(tmpDir)

		dstDir := filepath.Join(tmpDir, "fromdir")
		if err := env.CopyFrom(ctx, "/tmp/fromdir", dstDir); err != nil {
			t.Fatalf("CopyFrom() error = %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(dstDir, "a.txt"))
		if string(content) != "a" {
			t.Errorf("CopyFrom() a.txt = %q, want %q", string(content), "a")
		}
	})

	// Test Stop and Destroy
	t.Run("Stop", func(t *testing.T) {
		if err := env.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		if err := env.Destroy(ctx); err != nil {
			t.Errorf("Destroy() error = %v", err)
		}
		env = nil // prevent double destroy in defer
	})
}
