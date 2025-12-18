# Rollout

Rollout is a framework for evaluating and optimizing AI agents in container environments. It manages the complexity of defining containerized tasks, executing agent trials, and collecting results at scale.

## Features

- **Modular architecture**: Pluggable environment providers (Docker, Modal), agents, and verifiers
- **Concurrent execution**: Run multiple trials in parallel with configurable concurrency
- **Registry support**: Load tasks from local paths or remote git-based registries
- **Resource management**: Configure CPU, memory, and storage limits per task
- **Structured results**: JSON output with timing, rewards, and error details

## Installation

```bash
go install github.com/spachava753/rollout/cmd/rollout@latest
```

Or build from source:

```bash
git clone https://github.com/spachava753/rollout.git
cd rollout
go build -o rollout ./cmd/rollout
```

## Quick Start

### 1. Create a task

A task is a directory with:

```
my-task/
├── task.toml           # Configuration and metadata
├── instruction.md      # Instructions for the agent
├── environment/
│   └── Dockerfile      # Container environment definition
├── solution/
│   └── solve.sh        # Oracle solution (optional)
└── tests/
    └── test.sh         # Verification script
```

Example `task.toml`:

```toml
version = "1.0"

[metadata]
difficulty = "easy"
category = "programming"

[verifier]
timeout_sec = 120.0

[agent]
timeout_sec = 300.0

[environment]
cpus = 2
memory_mb = 4096
```

### 2. Create a job configuration

```yaml
# job.yaml
name: my-evaluation
jobs_dir: jobs
n_attempts: 3
n_concurrent_trials: 4
environment:
  type: docker
agents:
  - name: oracle
datasets:
  - path: ./my-task
```

### 3. Run the job

```bash
rollout job.yaml
```

Results are saved to `jobs/<job-name>/` with per-trial logs and a summary JSON.

## Configuration

### Job Configuration (`job.yaml`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | timestamp | Job name (used for output directory) |
| `jobs_dir` | string | `jobs` | Base directory for job output |
| `n_attempts` | int | 1 | Number of attempts per agent-task pair |
| `n_concurrent_trials` | int | 1 | Maximum parallel trial executions |
| `timeout_multiplier` | float | 1.0 | Multiplier for all timeouts |
| `instruction_path` | string | `/tmp/instruction.md` | Path where instruction is placed in container |
| `environment.type` | string | `docker` | Environment provider (`docker` or `modal`) |
| `environment.preserve_env` | string | `never` | When to keep environments (`never`, `always`, `on_failure`) |
| `verifier.override_timeout_sec` | float | - | Override verifier timeout for all tasks |
| `verifier.max_timeout_sec` | float | - | Maximum allowed verifier timeout |

### Task Configuration (`task.toml`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | string | `1.0` | Task format version |
| `environment.docker_image` | string | - | Pre-built image (skips Dockerfile build) |
| `environment.cpus` | int | 1 | CPU cores |
| `environment.memory_mb` | int | 2048 | Memory in MB |
| `environment.storage_mb` | int | 10240 | Storage in MB |
| `environment.build_timeout_sec` | float | 600 | Image build timeout |
| `agent.timeout_sec` | float | 600 | Agent execution timeout |
| `agent.install_timeout_sec` | float | 300 | Agent install script timeout |
| `verifier.timeout_sec` | float | 600 | Verification timeout |

## Environment Providers

### Docker (default)

Uses local Docker daemon. Requires Docker to be installed and running.

```yaml
environment:
  type: docker
```

### Modal

Uses [Modal](https://modal.com) cloud sandboxes for remote execution.

```yaml
environment:
  type: modal
  provider_config:
    region: us-east
```

**Requirements:**
- Modal CLI installed and authenticated (`pip install modal && modal setup`)
- Image builder version 2025.06+: `modal config set image_builder_version 2025.06`

**Limitations:**
- Dockerfiles cannot use `COPY` or `ADD` with local files (no build context support)
- Environment names are truncated to 64 characters

## Agents

### Oracle Agent

The built-in `oracle` agent executes the task's `solution/solve.sh` script. Use it to validate that tasks are solvable.

```yaml
agents:
  - name: oracle
```

### Custom Agents

Define custom agents with install and run scripts:

```yaml
agents:
  - name: my-agent
    install_script: |
      pip install my-agent-package
    run_script: |
      my-agent solve --instruction $ROLLOUT_INSTRUCTION_PATH
```

## Datasets

### Local Path

Load all tasks from a directory:

```yaml
datasets:
  - path: ./my-tasks
```

### Registry

Load specific tasks from a remote registry:

```yaml
datasets:
  - registry:
      url: https://example.com/registry.json
    name: dataset-name
    version: "1.0"
```

Registry format (`registry.json`):

```json
{
  "datasets": [
    {
      "name": "my-dataset",
      "version": "1.0",
      "tasks": [
        {
          "name": "task-1",
          "git_url": "https://github.com/org/repo.git",
          "git_commit_id": "abc123",
          "path": "tasks/task-1"
        }
      ]
    }
  ]
}
```

## Task Authoring Guide

### Directory Structure

```
my-task/
├── task.toml           # Required: configuration
├── instruction.md      # Required: agent instructions
├── environment/
│   └── Dockerfile      # Required: container definition
├── solution/
│   └── solve.sh        # Optional: oracle solution
└── tests/
    └── test.sh         # Required: verification script
```

### Writing Instructions

The `instruction.md` file is what the agent sees. Write clear, unambiguous instructions:

```markdown
# Task: Create a greeting file

Create a file called `hello.txt` in the current directory.
The file should contain exactly: `Hello, world!` (with a newline).

Do not include any additional text or formatting.
```

**Tips:**
- Be explicit about expected output format
- Specify file paths (absolute or relative to working directory)
- Include constraints and edge cases
- Avoid ambiguous language

### Environment Setup

Rollout expects **OS-like container images** with standard utilities. The Dockerfile should:

1. Use a full base image (e.g., `ubuntu:24.04`, not Alpine or distroless)
2. Ensure `bash` is available (used for script execution)
3. Set `WORKDIR` explicitly
4. Pre-install task-specific dependencies

```dockerfile
FROM ubuntu:24.04

# Install dependencies needed for the task
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
```

**Reserved paths in the container:**

| Path | Purpose |
|------|---------|
| `/tmp/instruction.md` | Task instruction (configurable via `instruction_path`) |
| `/tests/` | Verification scripts (copied at runtime) |
| `/oracle/` | Solution files (copied by oracle agent) |
| `/logs/agent/` | Agent can write logs here |
| `/logs/verifier/` | Verifier writes reward here |

### Writing Solutions

The `solution/solve.sh` script is executed by the oracle agent to validate the task is solvable:

```bash
#!/bin/bash
set -e

# Your solution here
echo "Hello, world!" > hello.txt
```

**Requirements:**
- Must be a bash script named `solve.sh`
- Should complete within the agent timeout
- Can include additional files in `solution/` directory

### Writing Verifiers

The verifier checks if the agent succeeded. It **must** write a reward value (0.0-1.0) to `/logs/verifier/reward.txt`.

**Simple bash verifier:**

```bash
#!/bin/bash
# tests/test.sh

mkdir -p /logs/verifier

if [ -f /app/hello.txt ] && grep -q "Hello, world!" /app/hello.txt; then
    echo "1.0" > /logs/verifier/reward
    echo "PASS: File exists with correct content"
else
    echo "0.0" > /logs/verifier/reward
    echo "FAIL: File missing or incorrect content"
    exit 1
fi
```

**Pytest verifier with partial credit:**

```python
# tests/test_state.py
import pytest
import os

def test_file_exists():
    assert os.path.exists("/app/hello.txt")

def test_content_correct():
    with open("/app/hello.txt") as f:
        assert f.read().strip() == "Hello, world!"

@pytest.fixture(scope="session", autouse=True)
def write_reward(request):
    yield
    failed = request.session.testsfailed
    total = request.session.testscollected
    reward = (total - failed) / total if total > 0 else 0.0
    os.makedirs("/logs/verifier", exist_ok=True)
    with open("/logs/verifier/reward.txt", "w") as f:
        f.write(str(reward))
```

Run pytest tests with a wrapper script:

```bash
#!/bin/bash
# tests/test.sh
cd /tests
python3 -m pytest test_state.py -v
```

### Pre-built Images

For faster iteration or complex environments, use a pre-built image instead of building from Dockerfile:

```toml
[environment]
docker_image = "myregistry/my-task-env:v1"
```

The image is pulled if not present locally. The `environment/Dockerfile` is ignored when `docker_image` is set.

### Testing Your Task

1. **Validate with oracle agent:**
   ```bash
   rollout job.yaml  # with oracle agent
   ```

2. **Check the output:**
   - `result.json` should show `reward: 1.0`
   - `verifier.log` should show passing tests

3. **Debug failures:**
   - Set `preserve_env: on_failure` to keep failed containers
   - Check `agent.log` and `verifier.log`

## Trial Lifecycle

Each trial executes through 6 phases:

1. **Setup**: Build or pull container image, create environment
2. **Install**: Run agent's install script (if defined)
3. **Execute**: Run agent against the task instruction
4. **Verify**: Copy tests and run verification script
5. **Collect**: Download logs and artifacts from container
6. **Teardown**: Destroy the environment

## Output Structure

```
jobs/
└── my-evaluation/
    ├── config.json           # Job configuration snapshot
    ├── results.json          # Aggregate results
    └── oracle/
        └── my-dataset/
            └── my-task__1/
                ├── result.json   # Trial result
                ├── agent.log     # Agent stdout/stderr
                └── verifier.log  # Verifier output
```

## Verifier Protocol

The verifier script must write a reward value (0.0-1.0) to `/logs/verifier/reward.txt`:

```bash
#!/bin/bash
# tests/test.sh
if [ -f /app/hello.txt ]; then
    echo "1.0" > /logs/verifier/reward
else
    echo "0.0" > /logs/verifier/reward
fi
```

Or use pytest with the reward file:

```python
# tests/test_state.py
import pytest

def test_solution():
    with open("/app/hello.txt") as f:
        assert f.read().strip() == "Hello, world!"

@pytest.fixture(scope="session", autouse=True)
def write_reward(request):
    yield
    failed = request.session.testsfailed
    total = request.session.testscollected
    reward = (total - failed) / total if total > 0 else 0.0
    with open("/logs/verifier/reward.txt", "w") as f:
        f.write(str(reward))
```

## Documentation

- [SPEC.md](SPEC.md) - Product specification with detailed concepts
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture and interfaces

## Development

```bash
# Run unit tests
go test -short ./...

# Run all tests (requires Docker)
go test -v ./...

# Build
go build -o rollout ./cmd/rollout
```

## License

MIT
