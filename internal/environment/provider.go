package environment

import (
	"context"
	"io"
	"time"
)

// Environment represents a running container environment.
type Environment interface {
	// ID returns the unique identifier for this environment.
	ID() string

	// CopyTo copies a local file or directory into the environment.
	CopyTo(ctx context.Context, src, dst string) error

	// CopyFrom copies a file or directory from the environment to local path.
	CopyFrom(ctx context.Context, src, dst string) error

	// Exec executes a command in the environment, streaming stdout and stderr to the provided writers.
	// Returns the exit code or error on failure.
	Exec(ctx context.Context, cmd string, stdout, stderr io.Writer, opts ExecOptions) (int, error)

	// Stop stops the environment but does not remove it.
	Stop(ctx context.Context) error

	// Destroy removes the environment and cleans up all resources.
	Destroy(ctx context.Context) error

	// Cost returns the cost incurred by this environment.
	Cost() float64
}

// ExecOptions configures command execution.
type ExecOptions struct {
	Env     map[string]string
	Timeout time.Duration
	WorkDir string
}

// Provider is a factory for creating environments.
type Provider interface {
	// Name returns the provider name (e.g., "docker", "modal", "k8s").
	Name() string

	// BuildImage builds a container image from the given context directory.
	BuildImage(ctx context.Context, opts BuildImageOptions) (string, error)

	// PullImage pulls a pre-built image from a registry.
	PullImage(ctx context.Context, imageRef string) error

	// CreateEnvironment creates and starts a new environment from an image.
	CreateEnvironment(ctx context.Context, opts CreateEnvironmentOptions) (Environment, error)
}

// BuildImageOptions configures image building.
type BuildImageOptions struct {
	ContextDir string
	Tag        string
	Timeout    time.Duration
	NoCache    bool
}

// CreateEnvironmentOptions configures environment creation.
type CreateEnvironmentOptions struct {
	ImageRef string
	CPUs     string
	Memory   string
	Storage  string
	Env      map[string]string
	Config   map[string]any
}
