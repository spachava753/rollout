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

- Kubernetes environment provider
- Retry logic for transient failures
- `preserve_env` policy enforcement

## Registry-Based Dataset Loading

The registry package (`internal/registry/`) is implemented with the following components:

- **types.go**: `RegistryTask`, `RegistryDataset`, and `cloneKey` types
- **loader.go**: `LoadFromPath()`, `LoadFromURL()`, `FindDataset()`
- **resolver.go**: `Resolver` that clones repos (deduplicated by git_url+commit) and loads tasks

The `dataset.Loader` has a new `LoadFromRegistry()` method.

### Clone Directory

Cloned repositories are stored in `/tmp/rollout-registry-<timestamp>/` and persist after job completion for debugging. Users should clean up manually if needed.

## Build Scripts

The project uses goyek v2 for build automation. Scripts are in `build/`.

```bash
go run ./build -h      # List available tasks
go run ./build vet     # Run go vet
```

For adding new tasks and detailed usage, see [docs/build.md](docs/build.md).

## Development

### Running Tests

```bash
# Fast unit tests (skips integration tests)
go test -short ./...

# All tests including integration tests (requires Docker)
go test -v ./internal/executor/...
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


## Known Limitations

### Modal Provider

The Modal provider does not support `COPY` or `ADD` instructions in Dockerfiles that reference local files. This is because the `modal-go` SDK builds images remotely and does not support uploading a local build context. Tasks must use self-contained images or pull artifacts from public URLs.
