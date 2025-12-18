package modal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modal-labs/libmodal/modal-go"
	"github.com/spachava753/rollout/internal/environment"
)

// ProviderConfig holds Modal-specific configuration.
type ProviderConfig struct {
	// AppName is the name of the Modal app to use. If empty, a unique name is generated.
	AppName string
	// Regions specifies the Modal regions (e.g., "us-east", "us-west").
	Regions []string
	// Verbose enables detailed sandbox logging.
	Verbose bool
}

// ParseProviderConfig extracts Modal-specific config from the generic config map.
func ParseProviderConfig(config map[string]any) ProviderConfig {
	pc := ProviderConfig{}
	if config == nil {
		return pc
	}
	if v, ok := config["app_name"].(string); ok {
		pc.AppName = v
	}
	if v, ok := config["region"].(string); ok {
		pc.Regions = []string{v}
	}
	if v, ok := config["regions"].([]any); ok {
		for _, r := range v {
			if s, ok := r.(string); ok {
				pc.Regions = append(pc.Regions, s)
			}
		}
	}
	if v, ok := config["verbose"].(bool); ok {
		pc.Verbose = v
	}
	return pc
}

// Provider implements the Modal environment provider using Modal Sandboxes.
type Provider struct {
	client *modal.Client
	config ProviderConfig
}

// MinImageBuilderVersion is the minimum required Modal image builder version.
// WORKDIR and other Dockerfile instructions require version 2025.06 or later.
const MinImageBuilderVersion = "2025.06"

// NewProvider creates a new Modal provider.
func NewProvider(config ProviderConfig) (*Provider, error) {
	if err := checkImageBuilderVersion(); err != nil {
		return nil, err
	}

	slog.Debug("initializing modal client")
	client, err := modal.NewClient()
	if err != nil {
		return nil, fmt.Errorf("creating modal client: %w", err)
	}
	return &Provider{
		client: client,
		config: config,
	}, nil
}

// ConfigReader reads Modal configuration.
type ConfigReader interface {
	ReadConfig() ([]byte, error)
}

// cliConfigReader reads config by executing the modal CLI.
type cliConfigReader struct{}

func (c *cliConfigReader) ReadConfig() ([]byte, error) {
	modalPath, err := exec.LookPath("modal")
	if err != nil {
		return nil, fmt.Errorf("modal CLI not found: %w", err)
	}
	cmd := exec.Command(modalPath, "config", "show")
	return cmd.Output()
}

// defaultConfigReader is the default ConfigReader used in production.
var defaultConfigReader ConfigReader = &cliConfigReader{}

// checkImageBuilderVersion verifies that the Modal image builder version is sufficient.
func checkImageBuilderVersion() error {
	return checkImageBuilderVersionWith(defaultConfigReader)
}

// checkImageBuilderVersionWith verifies the version using the provided ConfigReader.
func checkImageBuilderVersionWith(reader ConfigReader) error {
	output, err := reader.ReadConfig()
	if err != nil {
		return fmt.Errorf("failed to get modal config: %w", err)
	}

	var config struct {
		ImageBuilderVersion *string `json:"image_builder_version"`
	}
	if err := json.Unmarshal(output, &config); err != nil {
		return fmt.Errorf("failed to parse modal config: %w", err)
	}

	if config.ImageBuilderVersion == nil || *config.ImageBuilderVersion == "" {
		return fmt.Errorf("modal image_builder_version is not set; "+
			"WORKDIR support requires version %s or later. "+
			"Run: modal config set image_builder_version %s",
			MinImageBuilderVersion, MinImageBuilderVersion)
	}

	if *config.ImageBuilderVersion < MinImageBuilderVersion {
		return fmt.Errorf("modal image_builder_version %q is too old; "+
			"WORKDIR support requires version %s or later. "+
			"Run: modal config set image_builder_version %s",
			*config.ImageBuilderVersion, MinImageBuilderVersion, MinImageBuilderVersion)
	}

	slog.Debug("modal image builder version check passed", "version", *config.ImageBuilderVersion)
	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "modal"
}

// BuildImage builds a container image from the given context directory.
// For Modal, we return the context directory path as the "image reference".
// The actual image building happens lazily when the sandbox is created.
// LIMITATION: This provider does not support COPY/ADD instructions in Dockerfiles 
// that reference local files, as the modal-go SDK does not support build contexts.
// Images must be self-contained or use public URLs.
func (p *Provider) BuildImage(ctx context.Context, opts environment.BuildImageOptions) (string, error) {
	dockerfilePath := filepath.Join(opts.ContextDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return "", fmt.Errorf("Dockerfile not found at %s: %w", dockerfilePath, err)
	}
	slog.Debug("modal build deferred - using context directory", "context", opts.ContextDir)
	// Return context directory as the reference - we'll build in CreateEnvironment
	return opts.ContextDir, nil
}

// PullImage pulls a pre-built image from a registry.
// For Modal, this is a no-op since Modal handles image pulling internally.
func (p *Provider) PullImage(ctx context.Context, imageRef string) error {
	slog.Debug("modal pull is no-op - handled internally", "image", imageRef)
	return nil
}

// CreateEnvironment creates and starts a Modal sandbox.
func (p *Provider) CreateEnvironment(ctx context.Context, opts environment.CreateEnvironmentOptions) (environment.Environment, error) {
	// Determine app name: prefer opts.Name, then config, then generate
	appName := opts.Name
	if appName == "" {
		appName = p.config.AppName
	}
	if appName == "" {
		appName = fmt.Sprintf("rollout-%d", time.Now().UnixNano())
	}

	slog.Debug("creating modal app", "name", appName)
	
	// Get or create the Modal app
	app, err := p.client.Apps.FromName(ctx, appName, &modal.AppFromNameParams{
		CreateIfMissing: true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating modal app: %w", err)
	}

	// Build the image
	var image *modal.Image
	if isDockerContextPath(opts.ImageRef) {
		// ImageRef is a path to a directory with a Dockerfile
		slog.Debug("building modal image from dockerfile", "context", opts.ImageRef)
		image, err = p.buildImageFromDockerfile(ctx, app, opts.ImageRef)
		if err != nil {
			return nil, fmt.Errorf("building image from dockerfile: %w", err)
		}
	} else {
		// ImageRef is a registry image reference
		slog.Debug("using registry image for modal", "image", opts.ImageRef)
		image = p.client.Images.FromRegistry(opts.ImageRef, nil)
	}

	// Parse resource specs
	cpuCount := opts.CPUs
	if cpuCount <= 0 {
		cpuCount = 1
	}
	// Memory is already passed as MB
	memoryMiB := opts.MemoryMB
	if memoryMiB <= 0 {
		memoryMiB = 2048
	}

	// Build environment variables map including opts.Env
	envVars := make(map[string]string)
	for k, v := range opts.Env {
		envVars[k] = v
	}

	// Create sandbox parameters
	createParams := &modal.SandboxCreateParams{
		CPU:       float64(cpuCount),
		MemoryMiB: memoryMiB,
		Env:       envVars,
		Timeout:   24 * time.Hour, // Maximum allowed
		Verbose:   p.config.Verbose,
		Regions:   p.config.Regions,
	}

	slog.Debug("creating modal sandbox",
		"app", appName,
		"cpus", cpuCount,
		"memory_mib", memoryMiB,
		"regions", p.config.Regions)

	// Create the sandbox
	sandbox, err := p.client.Sandboxes.Create(ctx, app, image, createParams)
	if err != nil {
		return nil, fmt.Errorf("creating modal sandbox: %w", err)
	}

	slog.Debug("modal sandbox created", "sandbox_id", sandbox.SandboxID)

	return &ModalEnvironment{
		client:    p.client,
		sandbox:   sandbox,
		app:       app,
		appName:   appName,
		startTime: time.Now(),
		cpuCount:  cpuCount,
		memoryMiB: memoryMiB,
	}, nil
}

// buildImageFromDockerfile creates a Modal image from a Dockerfile.
func (p *Provider) buildImageFromDockerfile(ctx context.Context, app *modal.App, contextDir string) (*modal.Image, error) {
	dockerfilePath := filepath.Join(contextDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return nil, fmt.Errorf("reading Dockerfile: %w", err)
	}

	// Parse the Dockerfile to extract the base image and commands
	baseImage, commands, err := parseDockerfile(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing Dockerfile: %w", err)
	}

	slog.Debug("parsed dockerfile",
		"base_image", baseImage,
		"commands", len(commands))

	// Start with the base image
	image := p.client.Images.FromRegistry(baseImage, nil)

	// Apply Dockerfile commands
	if len(commands) > 0 {
		image = image.DockerfileCommands(commands, nil)
	}

	// Build the image eagerly so we catch build errors early
	slog.Debug("building modal image")
	builtImage, err := image.Build(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("building image: %w", err)
	}

	return builtImage, nil
}

// isDockerContextPath checks if the imageRef looks like a local directory path.
func isDockerContextPath(imageRef string) bool {
	if strings.HasPrefix(imageRef, "/") || strings.HasPrefix(imageRef, "./") || strings.HasPrefix(imageRef, "../") {
		info, err := os.Stat(imageRef)
		return err == nil && info.IsDir()
	}
	info, err := os.Stat(imageRef)
	return err == nil && info.IsDir()
}

// parseDockerfile extracts base image and commands from a Dockerfile.
func parseDockerfile(content string) (baseImage string, commands []string, err error) {
	lines := strings.Split(content, "\n")
	var currentCmd strings.Builder
	inContinuation := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Handle line continuations
		if inContinuation {
			currentCmd.WriteString(" ")
			if strings.HasSuffix(trimmed, "\\") {
				currentCmd.WriteString(strings.TrimSuffix(trimmed, "\\"))
			} else {
				currentCmd.WriteString(trimmed)
				commands = append(commands, currentCmd.String())
				currentCmd.Reset()
				inContinuation = false
			}
			continue
		}

		// Parse FROM instruction
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				baseImage = parts[1]
			}
			continue
		}

		// Parse Dockerfile instructions that Modal supports
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "RUN ") ||
			strings.HasPrefix(upper, "COPY ") ||
			strings.HasPrefix(upper, "ADD ") ||
			strings.HasPrefix(upper, "WORKDIR ") ||
			strings.HasPrefix(upper, "ENV ") ||
			strings.HasPrefix(upper, "USER ") ||
			strings.HasPrefix(upper, "EXPOSE ") ||
			strings.HasPrefix(upper, "LABEL ") {

			if strings.HasSuffix(trimmed, "\\") {
				currentCmd.WriteString(strings.TrimSuffix(trimmed, "\\"))
				inContinuation = true
			} else {
				commands = append(commands, trimmed)
			}
		}
	}

	if baseImage == "" {
		return "", nil, fmt.Errorf("no FROM instruction found in Dockerfile")
	}

	return baseImage, commands, nil
}

// parseCPUs converts a CPU string to a count.
func parseCPUs(cpus string) (int, error) {
	if cpus == "" {
		return 1, nil
	}
	var count float64
	if _, err := fmt.Sscanf(cpus, "%f", &count); err != nil {
		return 0, fmt.Errorf("invalid CPU value: %s", cpus)
	}
	result := int(count)
	if count > float64(result) {
		result++
	}
	if result < 1 {
		result = 1
	}
	return result, nil
}

// ModalEnvironment represents a running Modal sandbox.
type ModalEnvironment struct {
	client    *modal.Client
	sandbox   *modal.Sandbox
	app       *modal.App
	appName   string
	startTime time.Time
	cpuCount  int
	memoryMiB int
}

// ID returns the sandbox ID.
func (e *ModalEnvironment) ID() string {
	return e.sandbox.SandboxID
}

// CopyTo copies a local file or directory into the sandbox.
func (e *ModalEnvironment) CopyTo(ctx context.Context, src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	// Ensure destination directory exists via exec
	dstDir := filepath.Dir(dst)
	if dstDir != "/" && dstDir != "." {
		if _, err := e.execSimple(ctx, fmt.Sprintf("mkdir -p %q", dstDir)); err != nil {
			return fmt.Errorf("creating directory %s: %w", dstDir, err)
		}
	}

	slog.Debug("copying to modal sandbox",
		"sandbox_id", e.sandbox.SandboxID,
		"src", src,
		"dst", dst,
		"is_dir", info.IsDir())

	if info.IsDir() {
		return e.copyDirTo(ctx, src, dst)
	}
	return e.copyFileTo(ctx, src, dst)
}

// copyFileTo copies a single file to the sandbox.
func (e *ModalEnvironment) copyFileTo(ctx context.Context, src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}

	f, err := e.sandbox.Open(ctx, dst, "w")
	if err != nil {
		return fmt.Errorf("opening destination file: %w", err)
	}

	if _, err := f.Write(content); err != nil {
		f.Close()
		return fmt.Errorf("writing to destination: %w", err)
	}

	if err := f.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("flushing file: %w", err)
	}

	return f.Close()
}

// copyDirTo recursively copies a directory to the sandbox.
func (e *ModalEnvironment) copyDirTo(ctx context.Context, src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			_, err := e.execSimple(ctx, fmt.Sprintf("mkdir -p %q", dstPath))
			return err
		}

		return e.copyFileTo(ctx, path, dstPath)
	})
}

// CopyFrom copies a file or directory from the sandbox to local path.
func (e *ModalEnvironment) CopyFrom(ctx context.Context, src, dst string) error {
	slog.Debug("copying from modal sandbox",
		"sandbox_id", e.sandbox.SandboxID,
		"src", src,
		"dst", dst)

	// Check if source is a directory by trying to list it
	exitCode, _ := e.execSimple(ctx, fmt.Sprintf("test -d %q", src))
	if exitCode == 0 {
		return e.copyDirFrom(ctx, src, dst)
	}
	return e.copyFileFrom(ctx, src, dst)
}

// copyFileFrom copies a single file from the sandbox.
func (e *ModalEnvironment) copyFileFrom(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	f, err := e.sandbox.Open(ctx, src, "r")
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}

	content, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}

	if err := os.WriteFile(dst, content, 0644); err != nil {
		return fmt.Errorf("writing destination file: %w", err)
	}

	return nil
}

// copyDirFrom recursively copies a directory from the sandbox.
func (e *ModalEnvironment) copyDirFrom(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	// List directory contents using find command
	var stdout strings.Builder
	process, err := e.sandbox.Exec(ctx, []string{"find", src, "-maxdepth", "1", "-mindepth", "1"}, &modal.SandboxExecParams{})
	if err != nil {
		return fmt.Errorf("listing sandbox directory: %w", err)
	}

	io.Copy(&stdout, process.Stdout)
	if _, err := process.Wait(ctx); err != nil {
		return fmt.Errorf("waiting for find: %w", err)
	}

	entries := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, entry := range entries {
		if entry == "" {
			continue
		}

		baseName := filepath.Base(entry)
		dstPath := filepath.Join(dst, baseName)

		// Check if it's a directory
		exitCode, _ := e.execSimple(ctx, fmt.Sprintf("test -d %q", entry))
		if exitCode == 0 {
			if err := e.copyDirFrom(ctx, entry, dstPath); err != nil {
				return err
			}
		} else {
			if err := e.copyFileFrom(ctx, entry, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// execSimple runs a simple command and returns the exit code.
func (e *ModalEnvironment) execSimple(ctx context.Context, cmd string) (int, error) {
	process, err := e.sandbox.Exec(ctx, []string{"bash", "-c", cmd}, &modal.SandboxExecParams{})
	if err != nil {
		return -1, err
	}
	io.Copy(io.Discard, process.Stdout)
	io.Copy(io.Discard, process.Stderr)
	return process.Wait(ctx)
}

// Exec executes a command in the sandbox.
func (e *ModalEnvironment) Exec(ctx context.Context, cmd string, stdout, stderr io.Writer, opts environment.ExecOptions) (int, error) {
	execParams := &modal.SandboxExecParams{
		Env: opts.Env,
	}
	if opts.Timeout > 0 {
		execParams.Timeout = opts.Timeout
	}
	if opts.WorkDir != "" {
		execParams.Workdir = opts.WorkDir
	}

	// Truncate command for logging
	cmdPreview := cmd
	if len(cmdPreview) > 100 {
		cmdPreview = cmdPreview[:100] + "..."
	}
	slog.Debug("executing command in modal sandbox",
		"sandbox_id", e.sandbox.SandboxID,
		"command", cmdPreview,
		"timeout", opts.Timeout)

	process, err := e.sandbox.Exec(ctx, []string{"bash", "-c", cmd}, execParams)
	if err != nil {
		return -1, fmt.Errorf("executing command: %w", err)
	}

	// Stream stdout and stderr concurrently
	done := make(chan struct{}, 2)

	go func() {
		if stdout != nil {
			io.Copy(stdout, process.Stdout)
		} else {
			io.Copy(io.Discard, process.Stdout)
		}
		done <- struct{}{}
	}()

	go func() {
		if stderr != nil {
			io.Copy(stderr, process.Stderr)
		} else {
			io.Copy(io.Discard, process.Stderr)
		}
		done <- struct{}{}
	}()

	// Wait for streams to be fully consumed
	<-done
	<-done

	exitCode, err := process.Wait(ctx)
	if err != nil {
		return -1, fmt.Errorf("waiting for process: %w", err)
	}

	if exitCode != 0 {
		slog.Debug("command exited with non-zero code",
			"sandbox_id", e.sandbox.SandboxID,
			"exit_code", exitCode)
	}

	return exitCode, nil
}

// Stop stops the sandbox but does not remove it.
func (e *ModalEnvironment) Stop(ctx context.Context) error {
	slog.Debug("stopping modal sandbox", "sandbox_id", e.sandbox.SandboxID)
	return e.sandbox.Terminate(ctx)
}

// Destroy removes the sandbox and cleans up all resources.
func (e *ModalEnvironment) Destroy(ctx context.Context) error {
	slog.Debug("destroying modal sandbox", "sandbox_id", e.sandbox.SandboxID, "app", e.appName)
	
	// Terminate the sandbox first
	if err := e.sandbox.Terminate(ctx); err != nil {
		if !strings.Contains(err.Error(), "already terminated") &&
			!strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("terminating sandbox: %w", err)
		}
	}

	// Stop the Modal app to clean it up from the console.
	// The modal-go SDK doesn't expose AppStop on the public API, so we use the CLI.
	if err := e.stopApp(ctx); err != nil {
		return fmt.Errorf("stopping app: %w", err)
	}

	slog.Debug("modal sandbox destroyed", "sandbox_id", e.sandbox.SandboxID)
	return nil
}

// stopApp stops the Modal app using the modal CLI.
func (e *ModalEnvironment) stopApp(ctx context.Context) error {
	modalPath, err := exec.LookPath("modal")
	if err != nil {
		return fmt.Errorf("modal CLI not found: the modal-go SDK does not expose the AppStop API, " +
			"so the CLI is required to clean up apps. Install it with: pip install modal")
	}

	cmd := exec.CommandContext(ctx, modalPath, "app", "stop", e.appName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore errors if app is already stopped or not found
		outStr := string(output)
		if strings.Contains(outStr, "already stopped") ||
			strings.Contains(outStr, "not found") ||
			strings.Contains(outStr, "Could not find") {
			return nil
		}
		return fmt.Errorf("modal app stop failed: %s", outStr)
	}
	return nil
}

// Cost returns the cost incurred by this environment.
// Modal pricing (approximate, as of 2024):
// - CPU: ~$0.000463 per CPU-second
// - Memory: ~$0.000058 per GiB-second
func (e *ModalEnvironment) Cost() float64 {
	duration := time.Since(e.startTime).Seconds()
	cpuCost := duration * float64(e.cpuCount) * 0.000463
	memoryCost := duration * (float64(e.memoryMiB) / 1024.0) * 0.000058
	return cpuCost + memoryCost
}
