# Registry-Based Dataset Loading

The registry package (`internal/registry/`) enables loading tasks and datasets from remote Git repositories.

## Components

| File | Purpose |
|------|---------|
| types.go | `RegistryTask`, `RegistryDataset`, and `cloneKey` types |
| loader.go | `LoadFromPath()`, `LoadFromURL()`, `FindDataset()` |
| resolver.go | `Resolver` that clones repos (deduplicated by git_url+commit) and loads tasks |

The `dataset.Loader` has a `LoadFromRegistry()` method for registry-based loading.

## Clone Directory

Cloned repositories are stored in `/tmp/rollout-registry-<timestamp>/` and persist after job completion for debugging. Users should clean up manually if needed.

## Usage

Registry datasets are specified in job configurations with a `git_url` and optional `commit`:

```yaml
dataset:
  git_url: https://github.com/org/repo
  commit: abc123
  path: datasets/my-dataset
```
