package apple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// validatePath rejects paths containing ".." segments to prevent directory traversal.
func validatePath(path string) error {
	// Check original path for ".." segments (before normalization)
	// This catches both "../escape" and "/app/../etc" patterns
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("invalid path: contains directory traversal: %q", path)
		}
	}
	return nil
}

// runPipeline connects two commands with a pipe (cmd1.Stdout -> cmd2.Stdin) and waits for both.
// Returns combined error if either command fails.
func runPipeline(ctx context.Context, cmd1, cmd2 *exec.Cmd) error {
	// Create pipe
	r, w := io.Pipe()
	cmd1.Stdout = w
	cmd2.Stdin = r

	// Capture stderr for error messages
	var stderr1, stderr2 bytes.Buffer
	cmd1.Stderr = &stderr1
	cmd2.Stderr = &stderr2

	// Start both commands
	if err := cmd1.Start(); err != nil {
		return fmt.Errorf("starting first command: %w", err)
	}
	if err := cmd2.Start(); err != nil {
		cmd1.Process.Kill()
		return fmt.Errorf("starting second command: %w", err)
	}

	// Close writer when cmd1 finishes so cmd2's stdin gets EOF
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd1.Wait()
		w.Close()
	}()

	// Wait for cmd2 (consumer) first
	err2 := cmd2.Wait()
	r.Close()

	// Wait for cmd1's result (no race - using channel)
	err1 := <-errCh

	// Collect errors
	var errs []string
	if err1 != nil {
		errs = append(errs, fmt.Sprintf("command 1 failed: %v: %s", err1, stderr1.String()))
	}
	if err2 != nil {
		errs = append(errs, fmt.Sprintf("command 2 failed: %v: %s", err2, stderr2.String()))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// detectRuntimeUID determines the UID and GID to use for exec operations.
// Priority: explicit config > container inspect > exec id command > default "1000"
func detectRuntimeUID(ctx context.Context, containerID string, cfg ProviderConfig) (uid, gid string) {
	// Explicit override takes precedence
	if cfg.RuntimeUser != "" {
		uid = cfg.RuntimeUser
		gid = cfg.RuntimeGroup
		if gid == "" {
			gid = uid
		}
		slog.Debug("using configured runtime user", "uid", uid, "gid", gid)
		return uid, gid
	}

	// Try to get from container inspect
	uid, gid = detectFromInspect(ctx, containerID)
	if uid != "" {
		return uid, gid
	}

	// Fallback: exec id command
	uid, gid = detectFromExec(ctx, containerID)
	if uid != "" {
		return uid, gid
	}

	// Final fallback
	slog.Warn("could not detect runtime UID, defaulting to 1000")
	return "1000", "1000"
}

// detectFromInspect parses container inspect output to get User field.
func detectFromInspect(ctx context.Context, containerID string) (uid, gid string) {
	cmd := exec.CommandContext(ctx, "container", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		slog.Debug("container inspect failed", "error", err)
		return "", ""
	}

	// Parse JSON - inspect returns an array
	var inspectData []struct {
		Config struct {
			User string `json:"User"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &inspectData); err != nil || len(inspectData) == 0 {
		slog.Debug("failed to parse inspect output", "error", err)
		return "", ""
	}

	user := inspectData[0].Config.User
	if user == "" {
		// Empty user means root
		return "0", "0"
	}

	// Handle formats: "1000", "1000:1000", "username"
	if strings.Contains(user, ":") {
		parts := strings.SplitN(user, ":", 2)
		uid, gid = parts[0], parts[1]
	} else {
		uid = user
		gid = user
	}

	// Check if uid is numeric
	if !isNumeric(uid) {
		// Need to resolve username to UID
		resolved := resolveUsername(ctx, containerID, uid)
		if resolved != "" {
			uid = resolved
		}
	}
	if !isNumeric(gid) {
		gid = uid // Use same as UID if group is non-numeric
	}

	if uid != "" {
		slog.Debug("detected runtime user from inspect", "uid", uid, "gid", gid)
	}
	return uid, gid
}

// detectFromExec uses exec to run id command in the container.
func detectFromExec(ctx context.Context, containerID string) (uid, gid string) {
	// Get UID
	cmd := exec.CommandContext(ctx, "container", "exec", containerID, "id", "-u")
	output, err := cmd.Output()
	if err != nil {
		slog.Debug("exec id -u failed", "error", err)
		return "", ""
	}
	uid = strings.TrimSpace(string(output))

	// Get GID
	cmd = exec.CommandContext(ctx, "container", "exec", containerID, "id", "-g")
	output, err = cmd.Output()
	if err != nil {
		gid = uid // Default to UID
	} else {
		gid = strings.TrimSpace(string(output))
	}

	if uid != "" {
		slog.Debug("detected runtime user from exec", "uid", uid, "gid", gid)
	}
	return uid, gid
}

// resolveUsername resolves a username to UID inside the container.
func resolveUsername(ctx context.Context, containerID, username string) string {
	cmd := exec.CommandContext(ctx, "container", "exec", containerID, "id", "-u", username)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// isNumeric checks if a string contains only digits.
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
