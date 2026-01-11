# Rollout

Rollout is a framework for evaluating and optimizing agents in container environments.

## Documentation

| Document | Description |
|----------|-------------|
| [SPEC.md](SPEC.md) | Product specification: core concepts, configuration formats, behavior |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Technical architecture: interfaces, data models, package structure |
| [docs/development.md](docs/development.md) | Development guide: testing, building, test dataset |
| [docs/build.md](docs/build.md) | Build system: goyek tasks, adding new tasks |
| [docs/registry.md](docs/registry.md) | Registry-based dataset loading |

## Project Status

Initial implementation complete. The framework can execute tasks using the Docker environment provider and the Oracle agent.

**Implemented:** Core models, config parsing, task/dataset loading, Docker provider, trial executor, CLI entrypoint.

**Not yet implemented:** Kubernetes provider, retry logic, `preserve_env` policy.

## Quick Reference

```bash
# Run tests
go test -short ./...

# Build and run
go build -o rollout ./cmd/rollout
./rollout testdata/job.yaml

# Build tasks
go run ./build -h
```

## Code Conventions

- Follow ARCHITECTURE.md for package structure and interfaces
- Use `internal/` for all non-public packages
- Error types defined in `internal/models/errors.go`
- Config defaults in `internal/config/*.go`

## Known Limitations

### Modal Provider

The Modal provider does not support `COPY` or `ADD` instructions in Dockerfiles that reference local files. This is because the `modal-go` SDK builds images remotely and does not support uploading a local build context. Tasks must use self-contained images or pull artifacts from public URLs.
