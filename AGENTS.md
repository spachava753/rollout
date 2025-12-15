# Rollout

Rollout is a framework for evaluating and optimizing agents in container environments.

## Documentation

- **SPEC.md**: Product specification defining core concepts (tasks, datasets, agents, trials, jobs), configuration formats, and expected behavior.
- **ARCHITECTURE.md**: Technical architecture including Go interfaces, data models, data flow diagrams, and package structure.

## Project Status

Initial implementation complete. The framework can execute tasks using the Docker environment provider and the Oracle agent.

### What's Implemented

- **Models**: All core types (Task, Agent, Trial, Job, errors) in `internal/models/`
- **Config parsing**: task.toml and job.yaml parsing in `internal/config/`
- **Task/Dataset loading**: Load tasks from local filesystem in `internal/task/` and `internal/dataset/`
- **Docker provider**: Build images, create containers, exec commands, copy files in `internal/environment/docker/`
- **Trial executor**: Full 6-phase lifecycle (setup, install, execute, verify, collect, teardown) in `internal/executor/`
- **CLI entrypoint**: `cmd/rollout/main.go`

### Not Yet Implemented

- Registry-based dataset loading (remote git repos)
- Concurrent trial execution (fan-out/fan-in pattern)
- Modal and Kubernetes environment providers
- Result collector as separate component
- Retry logic for transient failures
- Cancellation handling (SIGINT/SIGTERM graceful shutdown)
- `preserve_env` policy enforcement
- Timeout multiplier for verifier overrides

## Development

### Running Tests

```bash
# Unit tests only
go test ./...

# Integration tests (requires Docker)
ROLLOUT_INTEGRATION_TEST=1 go test -v ./internal/executor/...
```

### Building

```bash
go build -o rollout ./cmd/rollout
./rollout testdata/job.yaml
```

### Test Dataset

`test-dataset/hello-world` is a minimal task for testing:
- Creates a file with "Hello, world!"
- Oracle solution in `solution/solve.sh`
- Pytest-based verification in `tests/`

### Test Job Config

`testdata/job.yaml` runs the Oracle agent against the test dataset.

## Code Conventions

- Follow ARCHITECTURE.md for package structure and interfaces
- Use `internal/` for all non-public packages
- Error types defined in `internal/models/errors.go`
- Config defaults in `internal/config/*.go`
