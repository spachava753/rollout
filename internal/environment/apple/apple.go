package apple

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spachava753/rollout/internal/environment"
)

// Provider implements the Apple Container environment provider.
type Provider struct {
	config ProviderConfig
}

// NewProvider creates a new Apple Container provider.
func NewProvider(cfg ProviderConfig) (*Provider, error) {
	// Check that container CLI is available
	if _, err := exec.LookPath("container"); err != nil {
		return nil, fmt.Errorf("apple container CLI not found: install from https://github.com/apple/container-tools or run: brew install container")
	}
	return &Provider{config: cfg}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "apple"
}

// BuildImage builds a container image using Apple Container.
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

	slog.Debug("executing container build",
		"tag", opts.Tag,
		"context", opts.ContextDir,
		"no_cache", opts.NoCache)

	cmd := exec.CommandContext(ctx, "container", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building container image: %w", err)
	}

	slog.Debug("container build completed", "tag", opts.Tag)
	return opts.Tag, nil
}

// PullImage pulls a pre-built image from a registry.
func (p *Provider) PullImage(ctx context.Context, imageRef string) error {
	slog.Debug("pulling container image", "image", imageRef)

	cmd := exec.CommandContext(ctx, "container", "image", "pull", imageRef)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling container image: %w", err)
	}

	slog.Debug("container image pulled", "image", imageRef)
	return nil
}

// CreateEnvironment creates and starts an Apple Container.
func (p *Provider) CreateEnvironment(ctx context.Context, opts environment.CreateEnvironmentOptions) (environment.Environment, error) {
	// Use provided name or generate one
	containerName := opts.Name
	if containerName == "" {
		containerName = fmt.Sprintf("rollout-%d", time.Now().UnixNano())
	}

	args := []string{
		"run",
		"-d",
		"--name", containerName,
	}

	// Add resource constraints
	if opts.CPUs > 0 {
		args = append(args, "--cpus", strconv.Itoa(opts.CPUs))
	}
	if opts.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", opts.MemoryMB))
	}

	// Add environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, opts.ImageRef)
	// Keep container running with sleep infinity
	args = append(args, "sleep", "infinity")

	slog.Debug("creating apple container",
		"name", containerName,
		"image", opts.ImageRef,
		"cpus", opts.CPUs,
		"memory_mb", opts.MemoryMB)

	cmd := exec.CommandContext(ctx, "container", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		// Handle name collision by appending timestamp
		if strings.Contains(errMsg, "name already in use") || strings.Contains(errMsg, "already exists") {
			containerName = fmt.Sprintf("%s-%d", containerName, time.Now().UnixNano())
			// Update args with new name
			for i, arg := range args {
				if arg == "--name" && i+1 < len(args) {
					args[i+1] = containerName
					break
				}
			}
			slog.Debug("retrying with unique name", "name", containerName)
			cmd = exec.CommandContext(ctx, "container", args...)
			stdout.Reset()
			stderr.Reset()
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return nil, fmt.Errorf("creating apple container: %w: %s", err, stderr.String())
			}
		} else {
			return nil, fmt.Errorf("creating apple container: %w: %s", err, errMsg)
		}
	}

	containerID := strings.TrimSpace(stdout.String())
	if containerID == "" {
		containerID = containerName // Some versions return empty, use name
	}

	slog.Debug("apple container created", "container_id", containerID)

	// Detect runtime UID after container is running
	uid, gid := detectRuntimeUID(ctx, containerID, p.config)

	return &Environment{
		containerID: containerID,
		runtimeUID:  uid,
		runtimeGID:  gid,
	}, nil
}

// Environment represents a running Apple Container.
type Environment struct {
	containerID string
	runtimeUID  string
	runtimeGID  string
	cost        float64
}

// ID returns the container ID.
func (e *Environment) ID() string {
	return e.containerID
}

// Exec executes a command in the container.
func (e *Environment) Exec(ctx context.Context, cmd string, stdout, stderr io.Writer, opts environment.ExecOptions) (int, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	args := []string{"exec"}

	// Run as container user if UID is set (not root unless specified)
	if e.runtimeUID != "" && e.runtimeUID != "0" {
		args = append(args, "--uid", e.runtimeUID)
	}

	// Add environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add working directory
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}

	args = append(args, e.containerID, "bash", "-c", cmd)

	// Truncate command for logging
	cmdPreview := cmd
	if len(cmdPreview) > 100 {
		cmdPreview = cmdPreview[:100] + "..."
	}
	slog.Debug("executing command in container",
		"container_id", e.containerID,
		"command", cmdPreview,
		"timeout", opts.Timeout)

	execCmd := exec.CommandContext(ctx, "container", args...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	err := execCmd.Run()
	if err != nil {
		// Try to extract exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			slog.Debug("command exited with non-zero code",
				"container_id", e.containerID,
				"exit_code", exitErr.ExitCode())
			return exitErr.ExitCode(), nil
		}
		// Check for context timeout
		if ctx.Err() == context.DeadlineExceeded {
			slog.Debug("command timed out", "container_id", e.containerID)
			return -1, fmt.Errorf("command timed out")
		}
		return -1, fmt.Errorf("executing command: %w", err)
	}

	return 0, nil
}

// CopyTo copies a local file or directory into the container using tar piping.
func (e *Environment) CopyTo(ctx context.Context, src, dst string) error {
	if err := validatePath(dst); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	slog.Debug("copying to container",
		"container_id", e.containerID,
		"src", src,
		"dst", dst,
		"is_dir", info.IsDir())

	// Create parent directory in container
	dstDir := filepath.Dir(dst)
	if dstDir != "/" && dstDir != "." {
		mkdirCmd := exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "mkdir", "-p", dstDir)
		if err := mkdirCmd.Run(); err != nil {
			return fmt.Errorf("creating directory %s: %w", dstDir, err)
		}
	}

	// Build tar command based on whether src is file or directory
	var tarCmd *exec.Cmd
	var extractPath string
	if info.IsDir() {
		// For directory: tar the contents, extract to dst
		tarCmd = exec.CommandContext(ctx, "tar", "-c", "-C", src, ".")
		extractPath = dst
		// Ensure destination directory exists
		mkdirCmd := exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "mkdir", "-p", dst)
		if err := mkdirCmd.Run(); err != nil {
			return fmt.Errorf("creating directory %s: %w", dst, err)
		}
	} else {
		// For file: tar the file, extract to parent dir
		tarCmd = exec.CommandContext(ctx, "tar", "-c", "-C", filepath.Dir(src), filepath.Base(src))
		extractPath = dstDir
	}

	// Build extract command - run as root to ensure we can write anywhere
	extractCmd := exec.CommandContext(ctx, "container", "exec", "-i", "-u", "root", e.containerID, "tar", "-xp", "-C", extractPath)

	// Run the pipeline
	if err := runPipeline(ctx, tarCmd, extractCmd); err != nil {
		return fmt.Errorf("copying to container: %w", err)
	}

	// Chown to runtime user
	chownPath := dst
	if !info.IsDir() {
		chownPath = filepath.Join(dstDir, filepath.Base(src))
	}
	chownCmd := exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "chown", "-R", e.runtimeUID+":"+e.runtimeGID, chownPath)
	if err := chownCmd.Run(); err != nil {
		slog.Debug("chown failed (may be expected for some files)", "error", err)
	}

	return nil
}

// CopyFrom copies a file or directory from the container to local path using tar piping.
func (e *Environment) CopyFrom(ctx context.Context, src, dst string) error {
	if err := validatePath(src); err != nil {
		return err
	}

	slog.Debug("copying from container",
		"container_id", e.containerID,
		"src", src,
		"dst", dst)

	// Check if src is a directory
	testCmd := exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "test", "-d", src)
	isDir := testCmd.Run() == nil

	// Create local parent directory
	if isDir {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return fmt.Errorf("creating local directory: %w", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("creating local directory: %w", err)
		}
	}

	// Build tar command from container
	var tarCmd *exec.Cmd
	var extractCmd *exec.Cmd
	if isDir {
		tarCmd = exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "tar", "-c", "-C", src, ".")
		extractCmd = exec.CommandContext(ctx, "tar", "-x", "-C", dst)
	} else {
		tarCmd = exec.CommandContext(ctx, "container", "exec", "-u", "root", e.containerID, "tar", "-c", "-C", filepath.Dir(src), filepath.Base(src))
		extractCmd = exec.CommandContext(ctx, "tar", "-x", "-C", filepath.Dir(dst))
	}

	// Run the pipeline
	if err := runPipeline(ctx, tarCmd, extractCmd); err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}

	// For single files, rename to match intended destination filename
	if !isDir {
		actualPath := filepath.Join(filepath.Dir(dst), filepath.Base(src))
		if actualPath != dst {
			if err := os.Rename(actualPath, dst); err != nil {
				return fmt.Errorf("renaming copied file: %w", err)
			}
		}
	}

	return nil
}

// Stop stops the container but does not remove it.
func (e *Environment) Stop(ctx context.Context) error {
	slog.Debug("stopping apple container", "container_id", e.containerID)

	cmd := exec.CommandContext(ctx, "container", "stop", e.containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errStr := string(output)
		// Ignore error if container already stopped or doesn't exist
		if !strings.Contains(errStr, "No such container") &&
			!strings.Contains(errStr, "already stopped") &&
			!strings.Contains(errStr, "not running") {
			return fmt.Errorf("stopping container: %w: %s", err, errStr)
		}
	}
	return nil
}

// Destroy removes the container and cleans up resources.
func (e *Environment) Destroy(ctx context.Context) error {
	slog.Debug("destroying apple container", "container_id", e.containerID)

	// Force remove the container
	cmd := exec.CommandContext(ctx, "container", "rm", "--force", e.containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errStr := string(output)
		// Ignore error if container already removed
		if !strings.Contains(errStr, "No such container") &&
			!strings.Contains(errStr, "not found") {
			return fmt.Errorf("removing container: %w: %s", err, errStr)
		}
	}
	return nil
}

// Cost returns the cost incurred by this environment (always 0 for local execution).
func (e *Environment) Cost() float64 {
	return e.cost
}
