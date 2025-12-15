# Rollout

Rollout is a framework for evaluating and optimizing agents in container environments. Rollout can be used in many ways like executing against custom agent evals, optimizing prompts, and CI/CD agent testing.

Defining and managing containerized tasks at scale is hard. Rollout is built to make it easy.

Rollout provides:

- Simple, modular interfaces for environments, agents, and tasks, making easy to extend
- All popular CLI agents pre-integrated
- A registry of popular benchmarks and datasets

## Core Concepts

Rollout has the following core concetps:

### Task

A task is a single instruction, container environment, and test script. Tasks are used to evaluate agents. A task is implemented as a directory of files in the following task format:

- **instruction.md**: The instruction is a markdown file that contains the task's instruction
- **task.toml**: The `task.toml` file contains the task's configuration and metadata. Metadata is arbitrary and can consist of any information a task developer wants. Config params are nested into their respective sections rather than flat.
  An example is shown below:

  ```toml
  version = "1.0"

  [metadata]
  author_name = "Steve Jobs"
  author_email = "steve@apple.com"
  difficulty = "easy"
  category = "programming"
  tags = ["trivial"]

  [verifier]
  timeout_sec = 120.0

  [agent]
  install_timeout_sec = 300.0
  timeout_sec = 120.0

  [environment]
  build_timeout_sec = 600.0
  docker_image = "some-org/some-name:some-tag"
  cpus = "1"
  memory = "2G"
  storage = "10G"
  ```

  Configuration parameters are defined by the following JSON Schema:

  ```json
  {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "Task Configuration",
    "type": "object",
    "required": ["version"],
    "properties": {
      "version": {
        "type": "string",
        "default": "1.0",
        "description": "Version of the task configuration format."
      },
      "source": {
        "type": ["string", "null"],
        "default": null,
        "description": "Optional source string for task provenance."
      },
      "metadata": {
        "type": "object",
        "description": "Arbitrary metadata provided by the task author.",
        "additionalProperties": true
      },
      "verifier": {
        "type": "object",
        "properties": {
          "timeout_sec": {
            "type": "number",
            "default": 600.0,
            "description": "Timeout in seconds for the verifier."
          }
        }
      },
      "agent": {
        "type": "object",
        "properties": {
          "install_timeout_sec": {
            "type": "number",
            "default": 300.0,
            "description": "Timeout in seconds for agent installation."
          },
          "timeout_sec": {
            "type": "number",
            "default": 600.0,
            "description": "Timeout in seconds for agent execution."
          }
        }
      },
      "environment": {
        "type": "object",
        "properties": {
          "build_timeout_sec": {
            "type": "number",
            "default": 600.0,
            "description": "Timeout in seconds for environment build."
          },
          "docker_image": {
            "type": ["string", "null"],
            "default": null,
            "description": "A pre-built Docker image to use for the environment. When set, this image is used instead of building from environment/Dockerfile. If the image is not present locally, Rollout pulls it from the registry before starting the container."
          },
          "cpus": {
            "type": "string",
            "default": "1",
            "description": "Number of CPUs available to the environment."
          },
          "memory": {
            "type": "string",
            "default": "2G",
            "description": "Amount of RAM available to the environment."
          },
          "storage": {
            "type": "string",
            "default": "10G",
            "description": "Amount of storage available to the environment."
          }
        }
      }
    }
  }
  ```

  The `environment.(cpus|memory|storage)` values are interpreted as a [quantity](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/) expression, similar to quantities specified in Kubernetes. Rollout translates these values to the appropriate configuration format expected by each environment provider (e.g., Docker resource constraints, Kubernetes resource requests/limits, Modal sandbox specs).

- **environment/**: The environment definition is placed in an `environment/` folder. **Rollout expects OS-like container images** with standard utilities available (bash, coreutils, etc.). Since tasks evaluate full-featured agents that may need to install dependencies, run build tools, or execute complex scripts, tasks typically build from base images like `ubuntu:24.04` or similar. Rollout assumes `bash` is available and uses it to execute agent install/execute scripts and the verifier. **Rollout does not require any specific file to exist in that directory**. Which file is required depends on the environment type being used for the evaluation. For example, to use docker, we check that an `environment/Dockerfile` is present. Different environment types could require other files to be present (e.g. an Apptainer environment could check for an `image.def` file). Most cloud sandbox providers only support `Dockerfile` defined environments and not docker compose.

  There are a few special paths in the environment's filesystem:

  | Path              | Description                                                                                                |
  | ----------------- | ---------------------------------------------------------------------------------------------------------- |
  | `/logs/verifier/` | Contains the reward file and other verifier logs.                                                          |
  | `/logs/agent/`    | A directory agents can use to store logs from their runs.                                                  |
  | `/oracle/`        | The solution folder is copied here by the Oracle agent at runtime and executed from the working directory. |
  | `/tests/`         | The tests folder is copied here by the Rollout harness at runtime and executed from the working directory. |

  The `/logs/` directory is downloaded to the host after the agent/verifier run and are often useful for debugging and analysis.

  **Working Directory:** The working directory for agent and verifier execution is determined by the task's `environment/Dockerfile`. If not explicitly set via `WORKDIR`, it inherits from the base image. Task authors should set `WORKDIR` explicitly to avoid ambiguity.

- **solution/**: The solution folder must contain a `solution/solve.sh` script. Other dependencies are allowed. This folder is copied to `/oracle` by the Oracle agent at runtime and executed from the working directory. If no solution is provided, the Oracle agent cannot be used to sanity check the task.
- **tests/**: The tests folder must contain a `tests/test.sh` script. The test script should install test dependencies and verify the agent completed the instruction. The test may be anything, from executing `pytest` to using LLM as a judge to verify task completion.

  Other dependencies are allowed in the `tests/` folder. This folder is copied to `/tests` by Rollout at runtime and executed from the working directory. E.g. `bash /tests/test.sh` is executed from `/app` in many cases.

  **We recommend using absolute paths in your test script to avoid relative path issues.**

  Importantly, the test script must produce a reward file in the `/logs/verifier/` directory **and exit with code 0**. This is the file that the verifier will read to determine if the task was successful. If the script exits with a non-zero code, the trial is marked as failed regardless of whether a reward file was produced.

  | Reward File                 | Format                            | Description                                                                                                 |
  | --------------------------- | --------------------------------- | ----------------------------------------------------------------------------------------------------------- |
  | `/logs/verifier/reward.txt` | Plain text (e.g. `1`, `0`, `0.5`) | A plain text file containing a single integer or float value, typically `1` for success or `0` for failure. |

  Your test script must output `reward.txt`.

  Often, a reward can be determined by the exit code or a unit test command.

  `tests/test.sh`:

  ```bash
  #!/bin/bash

  uvx pytest /tests/test.py

  if [ $? -eq 0 ]; then
    echo 1 > /logs/verifier/reward.txt
  else
    echo 0 > /logs/verifier/reward.txt
  fi
  ```

### Dataset

A dataset is a collection of tasks. Datasets are used to evaluate agents. Usually, a dataset corresponds to a benchmark (e.g. Terminal-Bench, SWE-Bench Verified, SWE-Bench Pro, etc.). Datasets can optionally be distributed via a registry.

Example structure of a dataset:

```shell
.
├── adaptive-rejection-sampler
│   ├── environment
│   │   ├── Dockerfile
│   │   └── warriors
│   │       ├── g2-clear.red
│   │       ├── paper.red
│   │       ├── snake.red
│   │       ├── stone.red
│   │       └── vampire.red
│   ├── instruction.md
│   ├── solution
│   │   └── solve.sh
│   ├── task.toml
│   └── tests
│       ├── test_outputs.py
│       └── test.sh
└── write-compressor
    ├── environment
    │   ├── data.txt
    │   ├── decomp.c
    │   ├── Dockerfile
    │   └── main.rs
    ├── instruction.md
    ├── solution
    │   └── solve.sh
    ├── task.toml
    └── tests
        ├── original-data.txt
        ├── original-decomp.c
        ├── test_outputs.py
        └── test.sh
```

A dataset may also defined in a `registry.json` file, which looks like:

```json
[
    {
        "name": "hello-world",
        "version": "head",
        "description": "A single, simple task for debugging.",
        "tasks": [
            {
                "name": "hello-world",
                "git_url": "https://github.com/laude-institute/harbor.git",
                "path": "examples/tasks/hello-world"
            }
        ]
    },
    {
        "name": "terminal-bench",
        "version": "2.0",
        "description": "Version 2.0 of Terminal-Bench, a benchmark for testing agents in terminal environments. More tasks, harder, and higher quality than 1.0.",
        "tasks": [
            {
                "name": "adaptive-rejection-sampler",
                "git_url": "https://github.com/laude-institute/terminal-bench-2.git",
                "git_commit_id": "69671fbaac6d67a7ef0dfec016cc38a64ef7a77c",
                "path": "adaptive-rejection-sampler"
            },
            {
                "name": "bn-fit-modify",
                "git_url": "https://github.com/laude-institute/terminal-bench-2.git",
                "git_commit_id": "69671fbaac6d67a7ef0dfec016cc38a64ef7a77c",
                "path": "bn-fit-modify"
            },
            ...
        ]
    }
]
```

A `registry.json` may contain multiple datasets, and each dataset may contain tasks that are from different repositories. Each task may also contain a path, and a git commit id.

Note that the dataset `version` field does not map to a specific artifact like git SHA or image digest, and merely acts as metadata at the moment. If there are multiple versions of a dataset, each version will have to be explicitly defined separately in a registry like so:

```json
[
  {
    "name": "example multi version dataset",
    "version": "1.0",
    "description": "A single, simple task for debugging.",
    "tasks": [
      {
        "name": "hello-world",
        "git_url": "https://github.com/laude-institute/harbor.git",
        "path": "examples/tasks/hello-world"
      }
    ]
  },
  {
    "name": "example multi version dataset",
    "version": "2.0",
    "description": "A single, simple task for debugging.",
    "tasks": [
      {
        "name": "hello-world",
        "git_url": "https://github.com/laude-institute/harbor.git",
        "path": "examples/tasks/hello-world"
      }
    ]
  },

  {
    "name": "example multi version dataset",
    "version": "alpha",
    "description": "A single, simple task for debugging.",
    "tasks": [
      {
        "name": "hello-world",
        "git_url": "https://github.com/laude-institute/harbor.git",
        "path": "examples/tasks/hello-world"
      }
    ]
  }
]
```

### Agent

An agent is a program that completes tasks. Agents are defined in the `job.yaml`. The agents block has the following structure:

```yaml
# ... other parts of job.yaml
agents:
  - name: Name of agent (Claude Code, CPE, Codex, Cline, etc.)
    description: Description of agent
    install: |
      Bash script to install agent
    execute: |
      Bash script to execute agent
    env: # key pair env vars that will passed to install and execute steps
      MY_API_KEY: ${MY_API_KEY} # can use env var from system
# ... other parts of job.yaml
```

When executing the agent, the environment variable `$ROLLOUT_TASK_INSTRUCTION` is set to the path where `instruction.md` has been copied in the container (default: `/tmp/instruction.md`). This path is configurable via `instruction_path` in `job.yaml`.

Here is an example agents definition for installing CPE.

```yaml
# ... other parts of job.yaml
agents:
  - name: cpe
    description: A custom, minimalistic harness that heavily customizable
    install: |
      #!/bin/bash
      set -euo pipefail

      log() {
          echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
      }

      log "Starting CPE installation..."

      log "Updating apt packages..."
      apt-get update
      apt-get install -y curl git
      log "✓ apt packages installed"

      log "Downloading Go 1.25.5..."
      # Force IPv4, increase retries/timeouts, handle transient network failures
      curl -4 -fSL --retry 10 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 300 \
        -o /tmp/go.tar.gz https://go.dev/dl/go1.25.5.linux-amd64.tar.gz || {
          log "Primary download failed, trying alternate mirror..."
          curl -4 -fSL --retry 10 --retry-delay 5 --retry-all-errors --connect-timeout 30 --max-time 300 \
            -o /tmp/go.tar.gz https://dl.google.com/go/go1.25.5.linux-amd64.tar.gz
      }
      # Verify download integrity
      if ! gzip -t /tmp/go.tar.gz 2>/dev/null; then
          log "ERROR: Downloaded file is corrupt"
          exit 1
      fi
      log "✓ Go tarball downloaded"

      log "Extracting Go..."
      tar -C /usr/local -xzf /tmp/go.tar.gz
      rm /tmp/go.tar.gz
      log "✓ Go installed to /usr/local/go"

      export PATH="/usr/local/go/bin:/root/go/bin:$PATH"
      echo 'export PATH="/usr/local/go/bin:/root/go/bin:$PATH"' >> ~/.bashrc
      log "✓ PATH configured"

      log "Installing CPE..."
      go install github.com/spachava753/cpe@latest
      log "✓ CPE installed"

      log "Creating config directory..."
      mkdir -p ~/.config/cpe
      log "✓ Config directory created"

      log "Writing CPE configuration..."
      cat > ~/.config/cpe/cpe.yaml << 'EOF'
      version: "1.0"

      models:
        - ref: sonnet
          display_name: "Claude Sonnet 4.5"
          id: claude-sonnet-4-5-20250929
          type: anthropic
          base_url: https://api.anthropic.com/
          api_key_env: ANTHROPIC_API_KEY
          context_window: 200000
          max_output: 64000
          input_cost_per_million: 3
          output_cost_per_million: 15
          generationDefaults:
            temperature: 1.0
            maxGenerationTokens: 64000

        - ref: opus
          display_name: "Claude Opus 4.5"
          id: claude-opus-4-5-20251101
          type: anthropic
          base_url: https://api.anthropic.com/
          api_key_env: ANTHROPIC_API_KEY
          context_window: 200000
          max_output: 64000
          input_cost_per_million: 5
          output_cost_per_million: 25
          generationDefaults:
            temperature: 1.0
            maxGenerationTokens: 64000

        - ref: haiku
          display_name: "Claude Haiku 4.5"
          id: claude-haiku-4-5-20251001
          type: anthropic
          base_url: https://api.anthropic.com/
          api_key_env: ANTHROPIC_API_KEY
          context_window: 200000
          max_output: 64000
          input_cost_per_million: 1
          output_cost_per_million: 5
          generationDefaults:
            temperature: 1.0
            maxGenerationTokens: 64000

      defaults:
        systemPromptPath: "/root/.config/cpe/agent_instructions.prompt"
        model: sonnet
        timeout: "30m"
        noStream: true
        codeMode:
          enabled: true
      EOF
      log "✓ CPE configuration written"

      log "Downloading system prompt template..."
      curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused \
        https://raw.githubusercontent.com/spachava753/cpe/main/examples/agent_instructions.prompt \
        -o ~/.config/cpe/agent_instructions.prompt
      log "✓ System prompt downloaded"

      log "CPE installation complete!"
    execute: |
      #!/bin/bash
      set -euo pipefail

      export PATH="/root/go/bin:/usr/local/go/bin:$PATH" && cpe -n -G --skip-stdin ${ROLLOUT_TASK_INSTRUCTION} 2>&1 | tee /logs/agent/cpe.txt
    env:
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
```

Note that when defining a [job](###Job), you may provide environment variables that are set before running the installation and execution script. This is especially helpful if you need certain inputs to execute the agent based on the job definition, such as a model id, or generation parameters.

In addition, `oracle` is a reserved agent name with special behavior. When an agent named `oracle` is specified, Rollout copies the task's `solution/` folder to `/oracle` in the container and executes `bash /oracle/solve.sh` from the working directory. No `install` or `execute` scripts are needed for the oracle agent—specifying `- name: oracle` in the agents list is sufficient.

### Container environment

Environments in Rollout are containers, typically defined as Docker images using a `Dockerfile` in a task `environment/` folder, as well as any artifacts that are used in the process of building the image to execute, such as dependencies or zip file artifacts that should be copied to the `Dockerfile`. The execution of the task in the container follows flow:

1. **Image building:** Build the image using the `environment/Dockerfile`. Some platforms like Modal and Fly can build on their platform, where as if the `environment.type` in `job.yaml` is docker, the image is built locally. **Built images are cached by default** for the Docker environment type; for cloud sandbox providers, caching depends on provider support and our implementation. The `force_build` option in `job.yaml` forces a rebuild, bypassing the cache. When `force_build` is set, Rollout builds from the task's `environment/Dockerfile` even if `environment.docker_image` is specified in `task.toml`.
2. **Start environment:** Start container with built image in platform (create sandbox via API calls with image, or deploy to Kubernetes as a Pod, with sleep comand) and keep it running. Copy the task's `instruction.md` to the configured path (default: `/tmp/instruction.md`) and set the `$ROLLOUT_TASK_INSTRUCTION` environment variable to this path.
3. **Install agent:** Copy the agent install script into the container and execute.
4. **Execute agent:** Copy the agent execute script into the container and execute. If the script exits with a non-zero exit code, the trial is marked as failed and the verifier is skipped. We cannot be certain of the filesystem state after a failed agent execution, so verification results would be unreliable. Additionally, verifiers can be expensive to run, so we only execute them when there are meaningful results to verify.
5. **Verify:** Copy the tasks `tests/` folder into the container at `/tests` and execute `/tests/test.sh`
6. **Create trial output:** Collect all outputs before stopping the container:
   - Copy the container's `/logs` directory to `<trial>/logs/` on the host
   - Write captured stdout/stderr from agent install to `<trial>/setup/`
   - Write captured stdout/stderr from agent execute to `<trial>/command/`
   - Generate `<trial>/result.json` with timing, cost, reward, and error information
7. **Stop environment:** Stop the execution of the running container.
8. **Clean up (optional):** Environment cleanup is controlled by the `preserve_env` setting:
   - `"never"` (default): Always clean up environment resources after task completion.
   - `"always"`: Never clean up; preserve all environments for inspection.
   - `"on_failure"`: Only preserve environments for failed trials (non-zero reward or error). This is recommended for debugging, as it preserves failed environments while cleaning up successful ones to save resources.
   
   Cleanup is provider-specific: for Docker, the container is removed (`docker rm <container-id>`); for Modal, the Modal app is deleted; for Kubernetes, related resources are deleted (e.g., if executing as a Job, the Job resource is removed). Note that this is separate from `force_build`, which controls image caching rather than environment cleanup.

**Error handling:** If a fatal error occurs at any phase (environment build, agent install/execution, verification, or teardown), the error details are written to `<trial>/error.txt` and the `error` field is populated in `result.json`. See [Error Types](#error-types) for all possible error types.

We aim to support many cloud providers and platforms out of the box, including Fly, Modal, and Kubernetes.

**Provider-specific configuration:** The `environment.provider_config` field accepts provider-specific settings. Each environment type may expose its own configuration knobs. Rollout provides sensible defaults where possible, but users may need to configure provider-specific options such as:
- **Docker:** Custom network, runtime (e.g., nvidia for GPU), volume mounts
- **Kubernetes:** Namespace, service account, node selectors, tolerations, resource classes
- **Modal:** App name prefix/suffix, region, secrets references
- **Fly:** Organization, region, VM size presets

Provider implementations should document their supported configuration options.

### Trial

A trial is an agent's attempt at completing a task. A trial is spawned from a job definition (not explicitly defined by the user) and produces a structured output including:

**Trial enumeration:** Trials are generated as the Cartesian product of (agent, task, attempt). For a job with `n_attempts` attempts, multiple agents, and multiple datasets containing tasks, Rollout generates one trial for each combination. Since trials typically run concurrently and complete in varying times, results are produced in non-deterministic order.

**Result generation:** Trial results are written to disk as each trial completes, rather than being collected and written at the end of the job. This ensures that partial results are preserved if a job is cancelled or encounters a fatal error.

Trial outputs include:

- **Reward:** The score produced by the verifier (from `/logs/verifier/reward.txt`)
- **Timing:** Durations and timestamps for each phase (environment setup, agent setup, agent execution, verification). Note that "environment setup" encompasses image build/pull, container start, and file copy operations.
- **Cost:** Resource costs incurred during the trial. Cost is provider-specific and calculated based on the environment type selected (e.g., Modal pricing for Modal environments, compute costs for Kubernetes, etc.)
- **Error:** Any errors that occurred during execution

Trial results are stored in `<trial>/result.json`. See [Job Output](#job-output) for the full schema and directory structure.

### Error Types

Trials can fail at various phases of execution. The `error` field in `result.json` uses a structured format to indicate the failure mode:

```json
{
  "type": "error_type_identifier",
  "message": "Human-readable description of what went wrong"
}
```

The following error types are defined:

| Error Type                               | Phase             | Description                                                                                 |
| ---------------------------------------- | ----------------- | ------------------------------------------------------------------------------------------- |
| `environment_build_failed`               | Environment Build | Dockerfile build failed (syntax error, failed command, missing base image)                  |
| `environment_build_timeout`              | Environment Build | Build exceeded `environment.build_timeout_sec`                                              |
| `environment_image_pull_failed`          | Environment Build | Failed to pull pre-built image specified in `environment.docker_image`                      |
| `environment_start_failed`               | Environment Start | Container failed to start after image was built/pulled                                      |
| `environment_resource_allocation_failed` | Environment Start | Platform could not allocate requested CPU/memory/storage                                    |
| `agent_install_failed`                   | Agent Install     | Agent install script returned non-zero exit code                                            |
| `agent_install_timeout`                  | Agent Install     | Agent install script exceeded `agent.install_timeout_sec`                                   |
| `agent_execution_failed`                 | Agent Execution   | Agent execute script returned non-zero exit code                                            |
| `agent_execution_timeout`                | Agent Execution   | Agent execution exceeded `agent.timeout_sec`                                                |
| `verifier_failed`                        | Verification      | Test script returned non-zero exit code                                                     |
| `verifier_timeout`                       | Verification      | Test script exceeded `verifier.timeout_sec`                                                 |
| `verifier_reward_missing`                | Verification      | Test script completed but no `/logs/verifier/reward.txt` was produced                       |
| `verifier_reward_invalid`                | Verification      | Reward file exists but contains invalid format (not a number)                               |
| `environment_teardown_failed`            | Teardown          | Failed to stop container or clean up resources                                              |
| `task_invalid`                           | Pre-execution     | Task structure is invalid (missing required files like `instruction.md` or `tests/test.sh`) |
| `task_not_found`                         | Pre-execution     | Task does not exist at the specified path                                                   |
| `internal_error`                         | Any               | Unexpected Rollout internal error                                                           |

**Error handling behavior:**

- If an error occurs during environment build/start, the trial is marked as failed and no further phases execute.
- If an error occurs during agent install/execution (including non-zero exit codes), the verifier is skipped and Rollout proceeds directly to teardown. This is intentional: the filesystem state after agent failure is uncertain, making verification unreliable.
- If the verifier fails or times out, the `reward` field is set to `null`.
- **If the verifier script exits with a non-zero exit code, it is always treated as an error (`verifier_failed`), even if a `reward.txt` file was produced.** The reward file is ignored in this case.
- Environment teardown errors are recorded but do not affect the trial's reward.
- For `task_invalid` and `task_not_found` errors, no container is started.

**Cancellation behavior:** Users may cancel a running job at any time (e.g., via SIGINT/Ctrl+C). When cancelled:
1. Rollout stops accepting new trials for execution.
2. Currently running trials are terminated: environments are stopped regardless of their execution phase.
3. Trials that were queued but not yet started are recorded as skipped in the job results.
4. The job-level `result.json` is written with the `cancelled` field set to `true` and `skipped_trials` listing the trials that were not executed.
5. Rollout exits with a non-zero exit code.

### Job

A job is a collection of trials. Jobs are used to evaluate agents across datasets. Jobs can be configured via `job.yaml` or `job.json` file. Jobs essentially map to a collection of trials.

The structure of job looks like this:

```yaml
name: my-eval-run # Optional. If not provided, defaults to YYYY-MM-DD__HH-mm-ss timestamp. Used as the output directory name.
jobs_dir: jobs # the output directory to store trial results
n_attempts: 1 # number of attempts per (agent, task) pair
n_concurrent_trials: 4 # number of concurrent trials
timeout_multiplier: 1.0 # multiplier applied to ALL configurable timeouts: task-level timeouts (agent.timeout_sec, agent.install_timeout_sec, verifier.timeout_sec, environment.build_timeout_sec), job-level override timeouts (verifier.override_timeout_sec, verifier.max_timeout_sec), and any other timeout settings. Useful when running on slower hardware
retry:
  max_attempts: 3 # maximum number of retry attempts for transient infrastructure failures (default: 3)
  initial_delay_ms: 1000 # initial delay in milliseconds before first retry (default: 1000)
  max_delay_ms: 30000 # maximum delay in milliseconds between retries (default: 30000)
  multiplier: 2.0 # exponential backoff multiplier (default: 2.0)
log_level: error # log level for rollout
instruction_path: /tmp/instruction.md # path where instruction.md is copied in the container; $ROLLOUT_TASK_INSTRUCTION will contain this path
environment:
  type: "docker" # or k8s, or modal, etc.
  force_build: false # if true, bypass image cache and force a rebuild. When set, builds from Dockerfile even if task specifies environment.docker_image
  preserve_env: "on_failure" # One of "always", "never", "on_failure". Default is "never". Controls when to preserve the environment after task completion.
  provider_config: # Provider-specific configuration. Structure depends on environment.type.
    # Example for Modal:
    # app_name: "rollout-my-app"  # Optional: Name of the Modal app to use
    # region: "us-east"           # Optional: Single region to run in
    # regions: ["us-east", "us-west"]  # Optional: Multiple regions
    # verbose: true               # Optional: Enable detailed sandbox logging
    # Example for Kubernetes:
    # namespace: "rollout-evals"
    # service_account: "rollout-runner"
    # node_selector:
    #   gpu: "true"
    # Example for Docker:
    # network: "rollout-net"
    # runtime: "nvidia"
  override_cpus: 1 # if set, override task specific cpu config
  override_memory: 2G # if set, override task specific memory config
  override_storage: 30G # if set, override task specific storage config
verifier:
  override_timeout_sec: 0 # if set, override task specific verifier timeout
  max_timeout_sec: 0 # if set, sets the ceiling of timeouts for verifiers
  disable: false # if set, disables executing verifier. Useful if you just want to collect trial execution traces
metrics:
  - type: "mean" # can be one of sum, min, max, or mean, will print the metrics as jobs are executing
agents:
  - name: "some-agent" # name of agent
    description: "description of some-agent" # name of agent
    install: "install bash script" # bash script to install agent
    execute: "execute bash script" # bash script to execute agent
    env:
      MY_API_KEY: ${MY_API_KEY}
  - name: oracle
datasets:
  - path: ./path-to-dataset # path to a local dataset, a folder that contains tasks
  - registry:
      path: ./path-to-registry # path to a registry defining datasets
    name: "dataset-name" # name of dataset as found in registry
    version: "dataset-version" # version of dataset as found in registry
  - registry:
      url: https://raw.githubusercontent.com/laude-institute/harbor/refs/heads/main/registry.json # url to a registry defining datasets
    name: "dataset-name" # name of dataset as found in registry
    version: "dataset-version" # version of dataset as found in registry
```

**Retry policy:** The `retry` configuration enables automatic retries for transient infrastructure failures such as HTTP 500 errors from cloud sandbox providers, network timeouts during image pulls, or temporary resource allocation failures. Retries use exponential backoff: the delay between attempts starts at `initial_delay_ms` and multiplies by `multiplier` after each attempt, capped at `max_delay_ms`. Only infrastructure-level failures are retried; agent execution failures and verifier failures are not retried as they represent legitimate task outcomes.

### Job Output

Running a job produces a structured output directory containing all trial results, logs, and metadata. The output is organized as follows:

| Path                                                                                    | Description                                                                                                            |
| --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `jobs/`                                                                                 | The jobs folder, as specified by `jobs_dir` in `job.yaml`                                                              |
| `jobs/<job-name>/`                                                                      | Directory for the job execution. If `name` is not provided in `job.yaml`, defaults to `YYYY-MM-DD__HH-mm-ss` timestamp |
| `jobs/<job-name>/config.json`                                                           | Copy of `job.yaml` in JSON format                                                                                      |
| `jobs/<job-name>/result.json`                                                           | Aggregate results of job execution                                                                                     |
| `jobs/<job-name>/<agent-name>/`                                                         | Results grouped by agent                                                                                               |
| `jobs/<job-name>/<agent-name>/<dataset-name>/`                                          | Results grouped by dataset within agent                                                                                |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/`                         | Trial-specific directory. `<task-name>` is the task folder name, `<n>` is the attempt number                           |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/result.json`              | Trial result                                                                                                           |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/error.txt`                | Error details if environment build/destroy or other fatal errors occurred                                              |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/setup/`                   | Rollout-captured logs from agent installation                                                                          |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/setup/stderr.txt`         | Stderr from agent installation                                                                                         |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/setup/stdout.txt`         | Stdout from agent installation                                                                                         |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/command/`                 | Rollout-captured logs from agent execution                                                                             |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/command/stderr.txt`       | Stderr from agent execution                                                                                            |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/command/stdout.txt`       | Stdout from agent execution                                                                                            |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/`                    | Copied from the container's `/logs` directory                                                                          |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/agent/`              | Agent-produced logs (from `/logs/agent/` in container)                                                                 |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/verifier/`           | Verifier logs and results (from `/logs/verifier/` in container)                                                        |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/verifier/reward.txt` | Reward file produced by tests                                                                                          |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/verifier/stderr.txt` | Stderr from test execution                                                                                             |
| `jobs/<job-name>/<agent-name>/<dataset-name>/<task-name>__<n>/logs/verifier/stdout.txt` | Stdout from test execution                                                                                             |

**Note:** Since a job may contain multiple datasets, it is possible for different datasets to contain tasks with the same name. To handle this, trial results are namespaced under the dataset name, allowing the same task name to exist in different datasets without conflict.

**Dataset naming:** For registry-defined datasets, the dataset name from the registry is used. For local datasets specified via `path`, the base name of the directory is used as the dataset name (e.g., a dataset at `./benchmarks/my-tasks` would use `my-tasks` as the dataset name in the output hierarchy).

#### Trial `result.json`

The trial-level `result.json` contains timing, cost, reward, and error information for a single trial. The `task_git_commit_id` field captures the Git commit SHA of the task source at execution time, enabling reproducibility and comparison across job runs. For registry-defined tasks, this is the `git_commit_id` from the registry entry; for local tasks, Rollout resolves the current HEAD commit of the task's repository (or `null` if the task is not in a Git repository):

```json
{
  "task_name": "hello-world",
  "dataset_name": "terminal-bench",
  "agent_name": "cpe",
  "attempt": 1,
  "task_git_commit_id": "a1b2c3d4e5f6789012345678901234567890abcd",
  "reward": 1.0,
  "cost": 0.0342,
  "error": null,
  "durations": {
    "total_sec": 245.3,
    "environment_setup_sec": 120.5,
    "agent_setup_sec": 45.2,
    "agent_execution_sec": 65.1,
    "verifier_sec": 14.5
  },
  "timestamps": {
    "started_at": "2025-01-15T10:30:00Z",
    "environment_setup_started_at": "2025-01-15T10:30:00Z",
    "environment_setup_ended_at": "2025-01-15T10:32:00Z",
    "agent_setup_started_at": "2025-01-15T10:32:00Z",
    "agent_setup_ended_at": "2025-01-15T10:32:45Z",
    "agent_execution_started_at": "2025-01-15T10:32:45Z",
    "agent_execution_ended_at": "2025-01-15T10:33:50Z",
    "verifier_started_at": "2025-01-15T10:33:50Z",
    "verifier_ended_at": "2025-01-15T10:34:05Z",
    "ended_at": "2025-01-15T10:34:05Z"
  }
}
```

The `error` field, when present, describes the failure type (see [Error Types](#error-types) for all possible values):

```json
{
  "task_name": "complex-task",
  "dataset_name": "terminal-bench",
  "agent_name": "cpe",
  "attempt": 1,
  "task_git_commit_id": "a1b2c3d4e5f6789012345678901234567890abcd",
  "reward": null,
  "cost": 0.012,
  "error": {
    "type": "agent_execution_timeout",
    "message": "Agent execution exceeded timeout of 600 seconds"
  },
  "durations": {
    "total_sec": 720.0,
    "environment_setup_sec": 115.2,
    "agent_setup_sec": 4.8,
    "agent_execution_sec": 600.0,
    "verifier_sec": null
  },
  "timestamps": {
    "started_at": "2025-01-15T11:00:00Z",
    "environment_setup_started_at": "2025-01-15T11:00:00Z",
    "environment_setup_ended_at": "2025-01-15T11:01:55Z",
    "agent_setup_started_at": "2025-01-15T11:01:55Z",
    "agent_setup_ended_at": "2025-01-15T11:02:00Z",
    "agent_execution_started_at": "2025-01-15T11:02:00Z",
    "agent_execution_ended_at": "2025-01-15T11:12:00Z",
    "verifier_started_at": null,
    "verifier_ended_at": null,
    "ended_at": "2025-01-15T11:12:00Z"
  }
}
```

#### Job `result.json`

The job-level `result.json` contains aggregate metrics across all trials:

```json
{
  "job_name": "my-eval-run",
  "cancelled": false,
  "total_trials": 20,
  "completed_trials": 18,
  "failed_trials": 2,
  "skipped_trials": 0,
  "pass_rate": 0.85,
  "mean_reward": 0.85,
  "total_cost": 1.234,
  "total_duration_sec": 3600.5,
  "started_at": "2025-01-15T10:00:00Z",
  "ended_at": "2025-01-15T11:00:00Z",
  "agents": {
    "cpe": {
      "total_trials": 10,
      "completed_trials": 9,
      "failed_trials": 1,
      "pass_rate": 0.9,
      "mean_reward": 0.9,
      "total_cost": 0.617
    },
    "oracle": {
      "total_trials": 10,
      "completed_trials": 9,
      "failed_trials": 1,
      "pass_rate": 0.8,
      "mean_reward": 0.8,
      "total_cost": 0.617
    }
  },
  "results": [
    {
      "task_name": "hello-world",
      "dataset_name": "terminal-bench",
      "agent_name": "cpe",
      "attempt": 1,
      "reward": 1.0
    },
    {
      "task_name": "hello-world",
      "dataset_name": "terminal-bench",
      "agent_name": "oracle",
      "attempt": 1,
      "reward": 1.0
    },
    {
      "task_name": "complex-task",
      "dataset_name": "terminal-bench",
      "agent_name": "cpe",
      "attempt": 1,
      "reward": 0.0
    },
    {
      "task_name": "complex-task",
      "dataset_name": "terminal-bench",
      "agent_name": "oracle",
      "attempt": 1,
      "reward": 1.0
    }
  ]
}
```

**Field definitions:**

- `cancelled`: Whether the job was cancelled before completion
- `skipped_trials`: Number of trials that were queued but not executed due to cancellation
- `completed_trials`: Trials where the verifier ran and produced a reward (regardless of reward value)
- `failed_trials`: Trials where an error prevented the verifier from producing a reward (any error type except `environment_teardown_failed`)
- `pass_rate`: Proportion of completed trials with reward 1.0
- `mean_reward`: Average reward across completed trials only (failed trials excluded)

## Usage

The user interacts with Rollout via a CLI, and simply just provides a `job.yaml` as an argument:

```shell
rollout job.yaml
```


## Future Considerations

The following features are currently out of scope but may be considered for future versions:

- **Job resumption:** Resuming stopped or cancelled jobs from where they left off. Currently, cancelled jobs must be restarted from scratch.
- **Advanced caching strategies:** Baking images with agent installations pre-applied to reduce trial startup time. Currently, agent installation runs fresh for each trial.
- **Network isolation:** Restricting agent network access during task execution. Currently, agents have full network access, which may be necessary for accessing documentation or downloading dependencies. Users should verify agents have not used unauthorized resources by reviewing agent logs.
- **Task filtering:** Selecting or filtering specific tasks from datasets in job configuration. Currently, all tasks in a dataset are included.
- **Anonymous datasets:** Executing a specified list of task paths directly without defining a dataset. Currently, tasks must be organized into a dataset structure.
