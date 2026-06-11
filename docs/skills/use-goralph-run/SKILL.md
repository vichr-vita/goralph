# Use goralph run for PRD-driven implementation

## When to use

Use for any project that should be implemented through `goralph run` from PRD tasks.

Do not use manual `goralph task start/pass` as the main implementation path unless debugging goralph itself.

## Core model

- goralph normally uses one shared SQLite DB for all projects.
- Current project is resolved from nearest Git root.
- DB rows are filtered by project/root path.
- Do not pass `--db` in normal use unless user/project config requires it.
- Use `--db .ralph/ralph.db` only for isolated tests, demos, sandboxes, or when you must avoid touching the shared goralph DB.

## PRD JSON schema

`prd.json` must be one top-level JSON array.

Each task must have exactly these fields:

```json
{
  "category": "feature",
  "description": "Implement add todo",
  "steps": [
    "User can type todo text",
    "User can submit todo",
    "New todo appears in the list"
  ],
  "passes": false
}
```

Rules:

- Top level = array only.
- No extra fields.
- `category` = non-empty string.
- `description` = non-empty string.
- `description` values must be unique within the file after trimming whitespace.
- `steps` = array of non-empty strings.
- `passes` = JSON boolean, not string or number.
- Use `passes: false` for new work.
- Use `passes: true` only when importing already-complete work.

Task-writing guidance:

- Write one behavior or deliverable per task.
- Make each task small enough for one `goralph run` turn.
- Order tasks by dependency.
- Use `steps` as acceptance criteria, not low-level implementation instructions.
- Include setup, core feature, tests, docs, and polish tasks when relevant.
- Prefer clear user-visible outcomes over vague engineering chores.

Minimal valid PRD:

```json
[
  {
    "category": "setup",
    "description": "Create project skeleton",
    "steps": [
      "Project has install and run commands",
      "App renders a title"
    ],
    "passes": false
  },
  {
    "category": "quality",
    "description": "Add verification",
    "steps": [
      "Automated tests cover core behavior",
      "Build command completes successfully"
    ],
    "passes": false
  }
]
```

## Procedure

1. Start in a clean Git worktree.
   ```sh
   git status --short
   ```

2. Initialize or confirm project metadata. Normal shared DB path is fine.
   ```sh
   goralph project info
   goralph project init --name <name> --description "<description>"
   ```

3. Write strict PRD JSON.
   - Top level = array.
   - Fields only: `category`, `description`, `steps`, `passes`.
   - Use small, ordered tasks because `goralph run` works one task per agent turn.

4. Validate and import PRD.
   ```sh
   goralph prd validate prd.json
   goralph prd import prd.json --replace
   goralph task list
   ```

5. Set feedback gates before run.
   ```sh
   goralph feedback set test "<test command>"
   goralph feedback set build "<build command>"
   goralph feedback list
   ```

6. Choose `--stale-after` intentionally.

   | Situation | Recommendation |
   | --- | --- |
   | Normal local work | omit flag; default `30m` |
   | Fast smoke test / demo | `--stale-after 5m` |
   | CI or scripted short run | `--stale-after 5m` |
   | Long agent task | `--stale-after 60m` or longer |
   | Recover known dead run | use chosen threshold plus `--force` |

   Notes:
   - `--stale-after` only decides when an active run looks stale.
   - It does not kill a live runner by itself.
   - Stale recovery still needs `--force`.
   - Do not use very tiny values like `2m` for normal runs unless deliberately stress-testing.

7. Run implementation.
   ```sh
   goralph run all --max-turns 6
   ```

   With explicit stale policy:
   ```sh
   goralph run all --max-turns 6 --stale-after 5m
   ```

8. If goralph reports dirty worktree after a run, inspect before acting.
   ```sh
   git status --short
   git diff
   ```

   Then choose one:
   ```sh
   # keep intentional leftovers
   git add -A
   git commit -m "Clean up goralph task output"

   # or discard accidental leftovers
   git restore <path>
   ```

   Then continue:
   ```sh
   goralph run all --max-turns 6
   ```

9. Confirm completion.
   ```sh
   goralph run all --max-turns 1
   ```

   Success output should be:
   ```text
   All tasks passed
   ```

10. Export final PRD state.
    ```sh
    goralph prd export prd.export.json
    goralph prd validate prd.export.json
    ```

## Isolated test variant

Use this only when you need no shared DB side effects:

```sh
mkdir -p .ralph
goralph --db .ralph/ralph.db project init --name <name> --description "<description>"
goralph --db .ralph/ralph.db prd validate prd.json
goralph --db .ralph/ralph.db prd import prd.json --replace
goralph --db .ralph/ralph.db run all --max-turns 6 --stale-after 5m
goralph --db .ralph/ralph.db prd export prd.export.json
```

## Pitfalls

- Most commands need a Git root. `prd validate` and `db` commands are projectless.
- `prd import` over existing tasks needs `--replace` or `--append` in non-interactive runs.
- `run all` needs clean worktree before and after each agent turn unless `--allow-dirty` is used.
- Avoid `--allow-dirty` unless deliberately debugging; it weakens safety.
- Nested runner may commit task work then leave formatting or generated-file diffs. Inspect, verify, and commit/revert before rerun.
- Do not use `--force` blindly. Inspect active run first with `goralph run list` or `goralph run show <id>`.
- `passes: true` exports only for tasks whose internal status is `passed`.

## Verification

```sh
goralph task list
goralph progress list
goralph feedback run test
goralph feedback run build
goralph prd export prd.export.json
git status --short
```

Success means all tasks passed, feedback passes, exported PRD has `passes: true`, and Git worktree is clean.
