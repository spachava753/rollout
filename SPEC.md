# Rollout

Rollout is a framework for evaluating and optimizing agents in container environments. Rollout can be used in many ways like building custom evals, optimizing prompts, running RL, generating SFT traces, and CI/CD agent testing.

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
  timeout_sec = 120.0

  [environment]
  build_timeout_sec = 600.0
  docker_image = "some-org/some-name:some-tag"
  cpus = "1"
  memory = "2048"
  storage = "10240"
  ```

  Configuration parameters:

  ```json
  {
  "version": {
    description: "Version of the task configuration format.",
    type: "string",
    default: "1.0",
    path: "version",
    required: true,
  },
  "metadata": {
    description: "Arbitrary metadata provided by the task author.",
    type: "object",
    path: "metadata"
  },
  "verifier.timeout_sec": {
    description: "Timeout in seconds for the verifier.",
    type: "number",
    default: 600.0,
    path: "verifier.timeout_sec"
  },
  "agent.timeout_sec": {
    description: "Timeout in seconds for the agent.",
    type: "number",
    default: 600.0,
    path: "agent.timeout_sec"
  },
  "environment.build_timeout_sec": {
    description: "Timeout in seconds for environment build.",
    type: "number",
    default: 600.0,
    path: "environment.build_timeout_sec"
  },
  "environment.docker_image": {
    description: "A pre-built Docker image to use for the environment.",
    type: 'string | null',
    default: null,
    path: "environment.docker_image"
  },
  "environment.cpus": {
    description: "Number of CPUs available to the environment.",
    type: "string",
    default: "1",
    path: "environment.cpus"
  },
  "environment.memory": {
    description: "Amount of RAM available to the environment.",
    type: "string",
    default: "2G",
    path: "environment.memory"
  },
  "environment.storage": {
    description: "Amount of storage available to the environment.",
    type: "string",
    default: "10G",
    path: "environment.storage"
  },
  "source": {
    description: "Optional source string for task provenance.",
    type: 'string | null',
    default: null,
    path: "source"
  }
  ```

  The `environment.(cpus|memory|storage)` values are interpreted as a [quantity](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/) expression, similar to quantities specified in Kubernetes.

- **environment/**: The environment definition is placed in an `environment/` folder. **Rollout does not require any specific file to exist in that directory**. Which file is required depends on the environment type being used for the evaluation. For example, to use docker, we check that an `environment/Dockerfile` or `environment/docker-compose.yaml` is present. Different environment types could require other files to be present (e.g. an Apptainer environment could check for an `image.def` file). Most cloud sandbox providers only support `Dockerfile` defined environments and not docker compose.

  There are a few special paths in the environment's filesystem:

  | Path              | Description                                                                                                |
  | ----------------- | ---------------------------------------------------------------------------------------------------------- |
  | `/logs/verifier/` | Contains the reward file and other verifier logs.                                                          |
  | `/logs/agent/`    | A directory agents can use to store logs from their runs.                                                  |
  | `/oracle/`        | The solution folder is copied here by the Oracle agent at runtime and executed from the working directory. |
  | `/tests/`         | The tests folder is copied here by the Rollout harness at runtime and executed from the working directory. |

  The `/logs/` directory is downloaded to the host after the agent/verifier run and are often useful for debugging and analysis.

- **solution/**: The solution folder must contain a `solution/solve.sh` script. Other dependencies are allowed. This folder is copied to `/oracle` by the Oracle agent at runtime and executed from the working directory. If no solution is provided, the Oracle agent cannot be used to sanity check the task.
- **tests/**: The tests folder must contain a `tests/test.sh` script. The test script should install test dependencies and verify the agent completed the instruction. The test may be anything, from executing `pytest` to using LLM as a judge to verify task completion.

  Other dependencies are allowed in the `tests/` folder. This folder is copied to `/tests` by Rollout at runtime and executed from the working directory. E.g. `bash /tests/test.sh` is executed from `/app` in many cases.

  **We recommend using absolute paths in your test script to avoid relative path issues.**

  Importantly, the test script must produce a reward file in the `/logs/verifier/` directory. This is the file that the verifier will read to determine if the task was successful.

  There are two ways to produce a reward file:

  | Reward File                  | Format                                                       | Description                                                                                                 |
  | ---------------------------- | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
  | `/logs/verifier/reward.txt`  | Plain text (e.g. `1`)                                        | A plain text file containing a single integer or float value, typically `1` for success or `0` for failure. |
  | `/logs/verifier/reward.json` | JSON (e.g. `{ "runtime_sec": 1.23, "accuracy": 0.95, ... }`) | A JSON file that can define multiple metrics as rewards, but they must be floats or integers.               |

  You may use either `reward.txt` or `reward.json` as the output of your test script. Rollout will read `reward.txt` by default and fall back to `reward.json`.

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

When executing the agent, the environment variable `$ROLLOUT_TASK_INSTRUCTION` is set, which contains the contents of `instruction.md` of the task.

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

In addition, there exists a special `oracle` agent, in which the `solution/` folder is copied to the `/oracle` path, and executed from the working directory.

### Container environment

Environments in Rollout are containers, typically defined as Docker images using a `Dockerfile` in a task `environment/` folder, as well as any artifacts that are used in the process of building the image to execute, such as dependencies or zip file artifacts that should be copied to the `Dockerfile`. The execution of the task in the container follows flow:

1. **Image building:** Build the image using the `environment/Dockerfile`. Some platforms like Modal and Fly can build on their platform, where as the if the `environment.type` in `job.yaml` is docker, the image is built locally.
2. **Start environment:** Start container with built image in platform (create sandbox via API calls with image, or deploy to Kubernetes as a Pod, with sleep comand) and keep it running. When starting, inject the `$ROLLOUT_TASK_INSTRUCTION` environment variable, which contains the task instruction.
3. **Install agent:** Copy the agent install script into the container and execute.
4. **Execute agent:** Copy the agent execute script into the container and execute.
5. **Verify:** Copy the tasks `tests/` folder into the container at `/tests` and execute `/tests/tests.sh`
6. **Create trial output:** Copy `/logs` folder to host. We copy this folder before stopping the container, as we may not have access to the file system after the container is stopped.
7. **Stop environment:** Stop the execution of the running container.
8. **Clean up (optional):** if delete is set to true (this is default), clean up resources like built images, pod in kubernetes, etc.

We aim to support many cloud providers and platforms out of the box, including Fly, Modal, and Kubernetes.

### Trial

A trial is an agent's attempt at completing a task. Essentially, a trial is a rollout that produces a reward. As such, a trial should contain info about the task, agent, the execution platform (Kubernetes, Modal, etc.) and a reward. A trial is explicitly defined by a user of Rollout, rather it is spawned from a job definition.

### Job

A job is a collection of trials. Jobs are used to evaluate agents across datasets. Jobs can be configured via `job.yaml` or `job.json` file. Jobs essentially map to a collection of trials.

The structure of job looks like this:

```yaml
name: Job name
jobs_dir: jobs # the output directory to store trial results
n_attempts: 1 # number of attempts
n_concurrent_trials: 4 # number of concurrent trials
timeout_multiplier: 1.0 # allows you to globally scale the timeout durations for various operations within a job. Useful if running older hardware, and need to increase timeouts specified for a task, agent execution, or verifier execution
log_level: error # log level for rollout
environment:
  type: "docker" # or k8s, or modal, etc.
  force_build: false # force a rebuild of an environment
  delete: true # default true, clean up the environment upon completion of task, such as removing images, snapshots, etc.
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

## Usage

The user interacts with Rollout via a CLI, and simply just provides a `job.yaml` as an argument:

```shell
rollout job.yaml
```
