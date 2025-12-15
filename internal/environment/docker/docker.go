package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spachava753/rollout/internal/environment"
)

// Provider implements the Docker environment provider.
type Provider struct{}

// NewProvider creates a new Docker provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "docker"
}

// BuildImage builds a Docker image from the given context directory.
func (p *Provider) BuildImage(ctx context.Context, opts environment.BuildImageOptions) (string, error) {
	args := []string{"build", "-t", opts.Tag}
	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, opts.ContextDir)

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building docker image: %w", err)
	}

	return opts.Tag, nil
}

// PullImage pulls a pre-built image from a registry.
func (p *Provider) PullImage(ctx context.Context, imageRef string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", imageRef)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling docker image: %w", err)
	}

	return nil
}

// CreateEnvironment creates and starts a Docker container.
func (p *Provider) CreateEnvironment(ctx context.Context, opts environment.CreateEnvironmentOptions) (environment.Environment, error) {
	// Use provided name or generate one
	containerID := opts.Name
	if containerID == "" {
		containerID = fmt.Sprintf("rollout-%d", time.Now().UnixNano())
	}

	args := []string{
		"run",
		"-d",
		"--name", containerID,
	}

	// Add resource constraints
	if opts.CPUs != "" {
		args = append(args, "--cpus", opts.CPUs)
	}
	if opts.Memory != "" {
		args = append(args, "--memory", opts.Memory)
	}

	// Add environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, opts.ImageRef)
	// Keep container running with sleep infinity
	args = append(args, "sleep", "infinity")

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("creating docker container: %w: %s", err, stderr.String())
	}

	return &DockerEnvironment{
		containerID: containerID,
	}, nil
}

// DockerEnvironment represents a running Docker container.
type DockerEnvironment struct {
	containerID string
	cost        float64
}

// ID returns the container ID.
func (e *DockerEnvironment) ID() string {
	return e.containerID
}

// CopyTo copies a local file or directory into the container.
func (e *DockerEnvironment) CopyTo(ctx context.Context, src, dst string) error {
	// Ensure dst directory exists
	dstDir := filepath.Dir(dst)
	if dstDir != "/" && dstDir != "." {
		mkdirCmd := exec.CommandContext(ctx, "docker", "exec", e.containerID, "mkdir", "-p", dstDir)
		if err := mkdirCmd.Run(); err != nil {
			return fmt.Errorf("creating directory %s: %w", dstDir, err)
		}
	}

	cmd := exec.CommandContext(ctx, "docker", "cp", src, fmt.Sprintf("%s:%s", e.containerID, dst))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying to container: %w: %s", err, stderr.String())
	}
	return nil
}

// CopyFrom copies a file or directory from the container to local path.
func (e *DockerEnvironment) CopyFrom(ctx context.Context, src, dst string) error {
	// Ensure dst directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "cp", fmt.Sprintf("%s:%s", e.containerID, src), dst)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying from container: %w: %s", err, stderr.String())
	}
	return nil
}

// Exec executes a command in the container.
func (e *DockerEnvironment) Exec(ctx context.Context, cmd string, stdout, stderr io.Writer, opts environment.ExecOptions) (int, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	args := []string{"exec"}

	// Add environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add working directory
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}

	args = append(args, e.containerID, "bash", "-c", cmd)

	execCmd := exec.CommandContext(ctx, "docker", args...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	err := execCmd.Run()
	if err != nil {
		// Try to extract exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		// Check for context timeout
		if ctx.Err() == context.DeadlineExceeded {
			return -1, fmt.Errorf("command timed out")
		}
		return -1, fmt.Errorf("executing command: %w", err)
	}

	return 0, nil
}

// Stop stops the container but does not remove it.
func (e *DockerEnvironment) Stop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", e.containerID)
	if err := cmd.Run(); err != nil {
		// Ignore error if container already stopped
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("stopping container: %w", err)
		}
	}
	return nil
}

// Destroy removes the container and cleans up resources.
func (e *DockerEnvironment) Destroy(ctx context.Context) error {
	// Force remove the container
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", e.containerID)
	if err := cmd.Run(); err != nil {
		// Ignore error if container already removed
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("removing container: %w", err)
		}
	}
	return nil
}

// Cost returns the cost incurred by this environment (always 0 for local Docker).
func (e *DockerEnvironment) Cost() float64 {
	return e.cost
}
