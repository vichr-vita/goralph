# goralph

[![CI](https://github.com/vichr-vita/goralph/actions/workflows/ci.yml/badge.svg)](https://github.com/vichr-vita/goralph/actions/workflows/ci.yml)
[![Release](https://github.com/vichr-vita/goralph/actions/workflows/release.yml/badge.svg)](https://github.com/vichr-vita/goralph/actions/workflows/release.yml)

`goralph` runs Ralph implementation loops for Git projects. It stores project state, PRD tasks, progress, and runner sessions in SQLite, then prompts an agent to work one PRD item at a time.

![goralph demo](assets/goralph.gif)

## Install or build

Install version `v0.1.4` from GitHub:

```sh
go install github.com/vichr-vita/goralph@v0.1.4
goralph --help
```

Remote `go install` needs a version suffix such as `@v0.1.4`.

If `goralph` is not found, add your Go binary directory to `PATH`.

For zsh:

```sh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

For bash:

```sh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

Update installed binary to `v0.1.4`:

```sh
go install github.com/vichr-vita/goralph@v0.1.4
```

Uninstall:

```sh
rm "$(go env GOPATH)/bin/goralph"
```

Build from this repository:

```sh
make test
make build
./target/goralph --help
```

Install from this checkout for local changes:

```sh
go install .
goralph --help
```

GitHub Actions builds release binaries when a `v*` tag is pushed. Create a new release:

```sh
git tag -a v0.1.4 -m "v0.1.4"
git push origin v0.1.4
```

The default runner command is `pi -p <generated-prompt>`. Use `--config`, user config, or project config when you need a different runner.

## Developer setup and quality commands

Install code-generation and migration tools into your Go bin directory:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Ensure `$(go env GOPATH)/bin` or your configured `GOBIN` is on `PATH`:

```sh
sqlc version
goose --version
```

Regenerate sqlc query code after changing `internal/db/sql/schema.sql`, `internal/db/sql/queries.sql`, or `sqlc.yaml`:

```sh
sqlc generate
```

Run development migrations through the goralph CLI so the same embedded Goose migrations used by the app are applied:

```sh
go run ./cmd/goralph --db .ralph/dev.db db migrate
```

For a fresh local development database:

```sh
go run ./cmd/goralph --db .ralph/dev.db db reset --force
```

Baseline verification before committing:

```sh
make test
```

Automated tests must not invoke real Pi. Use fake runner seams, temp configs, or runner command overrides for tests that exercise run flows.

## Configuration examples

goralph reads YAML config from `$XDG_CONFIG_HOME/goralph/config.yaml` or `~/.config/goralph/config.yaml`, then overlays the nearest `.ralph/config.yaml` found while walking up from the current directory. Use `--config <path>` to load one explicit config file instead of user/project discovery.

Example user config at `~/.config/goralph/config.yaml`:

```yaml
# Optional default SQLite database path.
# Use an absolute path; goralph does not expand shell `~` here.
db: /home/alice/.local/share/goralph/ralph.db

# Default Pi runner configuration. goralph appends the generated prompt.
runner:
  command: pi
  args:
    - -p

# Static feedback commands included in agent prompts.
feedback_commands:
  - gofmt -w .
  - go test ./...
```

Example project config at `.ralph/config.yaml`:

```yaml
# Project-local database override for this worktree.
# Relative paths resolve from the current working directory.
db: .ralph/ralph.db

# Same as default Pi runner, shown explicitly.
runner:
  command: pi
  args:
    - -p

# Alternative nested feedback command form.
feedback:
  commands:
    - go test ./...
    - go run ./cmd/goralph --help
```

Equivalent flat keys are also supported:

```yaml
runner_command: pi
runner_args: [-p]
feedback_command: go test ./...
```

Config precedence:

1. Built-in defaults: runner `pi` with args `[-p]`; database at `$XDG_DATA_HOME/goralph/ralph.db`, or `~/.local/share/goralph/ralph.db` when `XDG_DATA_HOME` is unset.
2. User config: `~/.config/goralph/config.yaml`.
3. Nearest project config: `.ralph/config.yaml`; project values override user values.
4. Environment variables such as `GORALPH_RUNNER_COMMAND` and `GORALPH_DB`.
5. CLI flags; `--config <path>` replaces user/project config discovery, and `--db <path>` wins for database path.

Database path override order is strict: `--db`, `GO_RALPH_DB`/`GORALPH_DB`, config `db`, `$XDG_DATA_HOME/goralph/ralph.db`, then `~/.local/share/goralph/ralph.db`. goralph creates the database parent directory when needed.

## Quickstart

Run commands from anywhere inside a Git worktree:

```sh
# Optional: see or initialize the auto-detected project.
goralph project info
goralph project init --name my-project --description "Short project note"

# Load PRD tasks into the current project.
goralph prd validate prd.json
goralph prd import prd.json --replace

# Inspect tasks.
goralph task list

# Run one agent turn, or keep running until stop condition.
goralph run one
goralph run all --max-turns 5

# Export current task state back to PRD JSON.
goralph prd export prd.json
```

Use `--db <path>` for an explicit SQLite database. Without it, goralph uses `GO_RALPH_DB`, then `$XDG_DATA_HOME/goralph/ralph.db`, then `~/.local/share/goralph/ralph.db`.

## Command reference

All commands accept global flags:

- `--config <path>` loads one YAML config file instead of user/project config discovery.
- `--db <path>` uses an explicit SQLite database path.
- `--json` emits JSON output where supported. For `run`, JSON mode streams newline-delimited run events and runner output events.
- `-h`, `--help` prints help.

Database path resolution order: `--db`, `GO_RALPH_DB`/`GORALPH_DB`, config `db`, `$XDG_DATA_HOME/goralph/ralph.db`, then `~/.local/share/goralph/ralph.db`.

Most commands are project-scoped and resolve the current project from the nearest Git root. `goralph db ...` and `goralph prd validate ...` do not require a Git project.

### Project commands

- `goralph project info` shows the auto-detected current project.
- `goralph project init [--name <name>] [--description <text>]` updates the current project metadata. The project row is created automatically from the Git root if missing.

### PRD commands

- `goralph prd validate <file>` validates a PRD JSON array. This command is projectless.
- `goralph prd import <file> [--replace|--append]` imports PRD tasks into the current project. `--replace` deletes current tasks first. `--append` adds imported tasks after existing tasks. Without either flag, interactive sessions ask before replacing existing tasks, while non-interactive sessions fail if tasks already exist.
- `goralph prd export [file]` exports current project tasks as PRD JSON. With no file, output goes to stdout.

### Task commands

Task statuses are `pending`, `in_progress`, `passed`, `failed`, and `blocked`.

- `goralph task list [--status <status>]` lists current project tasks, optionally filtered by status.
- `goralph task show <id>` shows one task with steps and latest progress.
- `goralph task add --category <name> --description <text> [--step <text> ...]` creates a pending task. Repeat `--step` to preserve step order.
- `goralph task update <id> [--category <name>] [--description <text>] [--status <status>] [--progress_report <text>] [--step <text> ...]` edits task fields. If any `--step` flag is present, all steps are replaced by the supplied ordered list.
- `goralph task start <id>` marks task `in_progress`.
- `goralph task pass <id>` marks task `passed`.
- `goralph task fail <id> --reason <text>` marks task `failed` and records the reason as progress.
- `goralph task block <id> --reason <text>` marks task `blocked` and records the reason as progress.
- `goralph task unblock <id>` marks task `pending` so it can be selected again.

### Progress commands

- `goralph progress add --summary <text> [--task <id>]` records project progress, optionally linked to a task. If a run is active, the entry also links to that run.
- `goralph progress list [--task <id>]` lists progress entries for the project or one task.

### Run commands and session inspection

Run commands accept persistent run flags:

- `--quiet` suppresses live runner output.
- `--allow-dirty` bypasses clean-worktree checks before and after agent turns.
- `--force` marks stale active runs failed before starting a new run.
- `--stale-after <duration>` controls stale active-run detection. Default: `30m0s`.

Commands:

- `goralph run one [--task <id>]` runs one agent turn. Without `--task`, goralph selects one eligible task. With `--task`, it targets that exact task.
- `goralph run all [--continue-on-blocked] [--max-turns <n>]` keeps running eligible turns. It does not support `--task`. `--continue-on-blocked` keeps running pending tasks when blocked or failed tasks remain. `--max-turns 0` means unlimited.
- `goralph run list` lists stored runs for the current project.
- `goralph run show <id>` shows run metadata and progress.
- `goralph run open <id>` opens the stored Pi session via `pi --session <session-ref>`.
- `goralph run export <id> [file]` exports the stored Pi session via `pi --export <session-ref> [file]`.

### Feedback commands

- `goralph feedback list` lists project feedback commands.
- `goralph feedback set <name> <command>` stores or replaces a named shell command for the project.
- `goralph feedback run [name]` runs one named feedback command, or all configured project feedback commands when `name` is omitted. Feedback commands run from the project root through `sh -c`.

Configured feedback commands are also included in generated agent prompts. User/project config may add static `feedback_commands`, `feedback.commands`, `feedback_command`, or `feedback.command` entries.

### DB commands

- `goralph db path` prints the resolved SQLite database path.
- `goralph db migrate` runs embedded migrations.
- `goralph db reset --force` deletes the database plus `-wal`/`-shm` files, then runs migrations. Reset refuses to run without `--force`.

## Project auto-detection

Most commands load the current project before they run.

goralph walks upward from the current directory to the nearest Git root. That Git root path is the project identity. If no project row exists yet, goralph creates one with the repository directory name as the default project name.

If no Git root can be found, project-scoped commands fail with a project resolution error. Run them inside a Git repository.

## PRD import and export workflow

PRD files use one strict top-level JSON array. Each array item must be an object with exactly these fields:

```json
[
  {
    "category": "documentation",
    "description": "Document the PRD JSON schema and example",
    "steps": [
      "Show the required JSON array structure",
      "Include a small valid PRD example"
    ],
    "passes": false
  }
]
```

Field meanings:

- `category`: required non-empty string used to group or label the task.
- `description`: required non-empty string and the task identity within the file.
- `steps`: required array of string acceptance steps. Each step must be non-empty.
- `passes`: required boolean completion marker.

Strict validation rules:

- The file must contain exactly one top-level JSON array and no trailing JSON value.
- Every array item must be an object.
- Only `category`, `description`, `steps`, and `passes` are allowed; unknown fields are rejected.
- `category` and `description` must be strings that are not blank after trimming whitespace.
- `steps` must be an array of strings; blank steps are rejected.
- `passes` must be a JSON boolean, not a string or number.
- `description` values must be unique within one imported file after trimming whitespace.

`passes` maps to internal task status during import and export:

- Importing `passes: true` creates the task with internal status `passed`.
- Importing `passes: false` creates the task with internal status `pending`.
- Export writes `passes: true` only for tasks whose internal status is `passed`.
- Export writes `passes: false` for `pending`, `in_progress`, `failed`, and `blocked` tasks.

Small valid PRD example:

```json
[
  {
    "category": "cli",
    "description": "Add version command",
    "steps": [
      "Print the current version",
      "Support JSON output"
    ],
    "passes": false
  }
]
```

Typical flow:

```sh
goralph prd validate prd.json
goralph prd import prd.json --replace
goralph task list
goralph run all
goralph prd export prd.json
```

Import behavior:

- `goralph prd import <file>` validates strict PRD JSON before writing.
- If current project has no tasks, import writes tasks directly.
- If tasks already exist in a TTY, default import asks before replacing them.
- If tasks already exist non-interactively, use `--replace` or `--append`.
- `--replace` deletes current project tasks, then imports file tasks.
- `--append` adds imported tasks after existing tasks.
- Imported task IDs come from the database, not PRD JSON.
- `passes: true` imports as `passed`; `passes: false` imports as `pending`.

Export behavior:

- `goralph prd export` writes pretty JSON to stdout.
- `goralph prd export <file>` writes pretty JSON to that file.
- Export includes category, description, ordered steps, and passes.
- Only internal `passed` status exports as `passes: true`; all other statuses export as `false`.

## Run one vs run all

`goralph run one` runs one agent turn.

- By default it selects from eligible tasks.
- Eligible tasks are `pending` and `failed`.
- `passed`, `blocked`, and `in_progress` tasks are not selected.
- Use `goralph run one --task <id>` to force one specific task.
- If no eligible task exists, output reports `No eligible task`.

`goralph run all` loops agent turns.

- It selects eligible tasks automatically; `--task` is not supported.
- It stops when all tasks are passed.
- It stops with an error when blocked or failed tasks remain by default.
- Use `--continue-on-blocked` to keep running pending tasks when blocked or failed tasks exist.
- Use `--max-turns <n>` to cap turns; `0` means unlimited.
- It stops when the agent prints `<promise>COMPLETE</promise>`.

Run safety:

- Runs require a clean Git worktree before each agent turn.
- Use `--allow-dirty` to bypass the pre-run dirty-worktree guard.
- After an agent turn, goralph expects a clean committed state unless dirty runs are allowed.
- A second active run in the same project is rejected. Use `goralph run show <id>` to inspect active or past runs.

## Agent progress and task update contract

The generated agent prompt gives the agent project context, task context, recent progress, and feedback command names. The agent must work one feature only.

For each selected task, the agent contract is:

```sh
goralph task start <task-id>
# make code or doc changes
# run relevant feedback loops, for example: go test ./...
goralph progress add --task <task-id> --summary "what changed and what passed"
# finish with exactly one final task state:
goralph task pass <task-id>
goralph task fail <task-id> --reason "why it failed"
goralph task block <task-id> --reason "what blocks it"
# commit the feature
# when all work is complete, print:
# <promise>COMPLETE</promise>
```

Progress entries are stored on the current project. When an active run exists, progress also links to that run. `fail` and `block` require a reason and record that reason as task progress.

Agents should run feedback before passing a task. If named feedback commands exist, prefer `goralph feedback run <name>`. goralph v1 records guidance, but does not run or enforce feedback after the agent exits.
