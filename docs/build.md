# Build System

Rollout uses [goyek](https://github.com/goyek/goyek) v2 for build automation. Goyek is a Go-native task runner that keeps build logic in Go, avoiding shell script complexity and cross-platform issues.

## Running Build Tasks

From the project root:

```bash
# Run a specific task
go run ./build <task>

# Show available tasks
go run ./build -h

# Run multiple tasks
go run ./build task1 task2
```

## Available Tasks

| Task | Description |
|------|-------------|
| vet  | Run go vet on all packages |

## Adding New Tasks

Tasks are defined in `build/main.go` using `goyek.Define()`:

```go
var myTask = goyek.Define(goyek.Task{
    Name:  "my-task",
    Usage: "Description of what this task does",
    Action: func(a *goyek.A) {
        // Task implementation
        cmd := exec.Command("go", "test", "./...")
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        if err := cmd.Run(); err != nil {
            a.Error(err)
        }
    },
})
```

### Task Dependencies

Tasks can depend on other tasks:

```go
var build = goyek.Define(goyek.Task{
    Name:  "build",
    Usage: "Build the binary",
    Deps:  goyek.Deps{vet}, // Runs vet first
    Action: func(a *goyek.A) {
        // build logic
    },
})
```

### Setting a Default Task

```go
func init() {
    goyek.SetDefault(build)
}
```

## Common Patterns

### Running shell commands

```go
cmd := exec.Command("go", "build", "-o", "rollout", "./cmd/rollout")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
if err := cmd.Run(); err != nil {
    a.Error(err)
}
```

### Failing a task

```go
a.Error("something went wrong")  // Marks task as failed, continues
a.Fatal("critical error")        // Marks task as failed, stops immediately
```

### Skipping a task

```go
if someCondition {
    a.Skip("skipping because...")
}
```

## References

- [goyek documentation](https://github.com/goyek/goyek)
- [goyek/x extensions](https://github.com/goyek/x) - additional utilities like `cmd.Exec`
