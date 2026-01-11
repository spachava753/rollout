# Development Guide

## Running Tests

```bash
# Fast unit tests (skips integration tests)
go test -short ./...

# All tests including integration tests (requires Docker)
go test -v ./internal/executor/...
```

## Building

```bash
go build -o rollout ./cmd/rollout
./rollout testdata/job.yaml
```

## Test Dataset

`test-dataset/hello-world` is a minimal task for integration testing:

- Creates a file with "Hello, world!"
- Oracle solution in `solution/solve.sh`
- Pytest-based verification in `tests/`

## Test Job Config

`testdata/job.yaml` runs the Oracle agent against the test dataset.

## Build Scripts

The project uses goyek v2 for build automation. See [build.md](build.md) for details.

```bash
go run ./build -h      # List available tasks
go run ./build vet     # Run go vet
```
