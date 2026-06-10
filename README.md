# goralph

`goralph` runs Ralph implementation loops for Git projects. It stores project state, PRD tasks, progress, and runner sessions in SQLite, then prompts an agent to work one PRD item at a time.

## Install or build

From this repository:

```sh
go test ./...
go build -o ./bin/goralph ./cmd/goralph
./bin/goralph --help
```

To install into your Go bin directory:

```sh
go install ./cmd/goralph
goralph --help
```

The default runner command is `pi -p <generated-prompt>`. Use `--config`, user config, or project config when you need a different runner.

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

## Project auto-detection

Most commands load the current project before they run.

goralph walks upward from the current directory to the nearest Git root. That Git root path is the project identity. If no project row exists yet, goralph creates one with the repository directory name as the default project name.

If no Git root can be found, project-scoped commands fail with a project resolution error. Run them inside a Git repository.

## PRD import and export workflow

PRD files are JSON arrays. Each item has `category`, `description`, `steps`, and `passes`.

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
