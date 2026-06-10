-- name: Ping :one
SELECT 1;

-- name: GetProjectByRootPath :one
SELECT id, name, root_path, description, created_at, updated_at
FROM project
WHERE root_path = ?;

-- name: CreateProject :one
INSERT INTO project (name, root_path)
VALUES (?, ?)
RETURNING id, name, root_path, description, created_at, updated_at;

-- name: UpdateProject :one
UPDATE project
SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, name, root_path, description, created_at, updated_at;

-- name: CountTasksByProject :one
SELECT COUNT(*) FROM task WHERE project_id = ?;

-- name: ListTasksByProject :many
SELECT id, project_id, category, description, status, progress_report, created_at, updated_at
FROM task
WHERE project_id = ?
ORDER BY id;

-- name: ListTasksByProjectAndStatus :many
SELECT id, project_id, category, description, status, progress_report, created_at, updated_at
FROM task
WHERE project_id = ? AND status = ?
ORDER BY id;

-- name: GetTaskByProjectAndID :one
SELECT id, project_id, category, description, status, progress_report, created_at, updated_at
FROM task
WHERE project_id = ? AND id = ?;

-- name: GetNextEligibleTaskByProject :one
SELECT id, project_id, category, description, status, progress_report, created_at, updated_at
FROM task
WHERE project_id = ? AND status IN ('pending', 'failed')
ORDER BY id
LIMIT 1;

-- name: ListTaskStepsByTask :many
SELECT id, task_id, position, description, created_at, updated_at
FROM task_step
WHERE task_id = ?
ORDER BY position;

-- name: ListLatestProgressByTask :many
SELECT id, project_id, task_id, run_id, summary, created_at, updated_at
FROM progress
WHERE task_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListTaskPRDRowsByProject :many
SELECT
    t.id,
    t.category,
    t.description,
    t.status,
    ts.description AS step_description
FROM task t
LEFT JOIN task_step ts ON ts.task_id = t.id
WHERE t.project_id = ?
ORDER BY t.id, ts.position;

-- name: CreateTask :one
INSERT INTO task (project_id, category, description, status)
VALUES (?, ?, ?, ?)
RETURNING id, project_id, category, description, status, progress_report, created_at, updated_at;

-- name: UpdateTask :one
UPDATE task
SET category = ?, description = ?, status = ?, progress_report = ?, updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ?
RETURNING id, project_id, category, description, status, progress_report, created_at, updated_at;

-- name: CreateTaskStep :one
INSERT INTO task_step (task_id, position, description)
VALUES (?, ?, ?)
RETURNING id, task_id, position, description, created_at, updated_at;

-- name: CreateProgress :one
INSERT INTO progress (project_id, task_id, run_id, summary)
VALUES (?, ?, ?, ?)
RETURNING id, project_id, task_id, run_id, summary, created_at, updated_at;

-- name: CreateRun :one
INSERT INTO run (project_id, task_id, runner_name, status, host, started_at, heartbeat_at)
VALUES (?, ?, ?, 'running', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at;

-- name: UpdateRunProcess :one
UPDATE run
SET pid = ?,
    host = ?,
    heartbeat_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ? AND status = 'running'
RETURNING id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at;

-- name: UpdateRunHeartbeat :exec
UPDATE run
SET heartbeat_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ? AND status = 'running';

-- name: MarkRunFailed :one
UPDATE run
SET status = 'failed',
    exit_error = ?,
    heartbeat_at = CURRENT_TIMESTAMP,
    finished_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ? AND status = 'running'
RETURNING id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at;

-- name: FinishRun :one
UPDATE run
SET runner_name = ?,
    runner_version = ?,
    runner_model = ?,
    session_id = ?,
    session_path = ?,
    status = ?,
    exit_code = ?,
    exit_signal = ?,
    exit_error = ?,
    pid = ?,
    host = ?,
    heartbeat_at = CURRENT_TIMESTAMP,
    finished_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ?
RETURNING id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at;

-- name: SetRunTaskID :one
UPDATE run
SET task_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE project_id = ? AND id = ?
RETURNING id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at;

-- name: GetActiveRunByProject :one
SELECT id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at
FROM run
WHERE project_id = ? AND status = 'running'
ORDER BY COALESCE(started_at, created_at) DESC, id DESC
LIMIT 1;

-- name: GetRunByProjectAndID :one
SELECT id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at
FROM run
WHERE project_id = ? AND id = ?;

-- name: ListRunsByProject :many
SELECT id, project_id, task_id, runner_name, runner_version, runner_model, session_id, session_path, status, exit_code, exit_signal, exit_error, pid, host, heartbeat_at, started_at, finished_at, created_at, updated_at
FROM run
WHERE project_id = ?
ORDER BY COALESCE(started_at, created_at) DESC, id DESC;

-- name: ListProgressByRun :many
SELECT id, project_id, task_id, run_id, summary, created_at, updated_at
FROM progress
WHERE project_id = ? AND run_id = ?
ORDER BY created_at DESC, id DESC;

-- name: ListProgressByProject :many
SELECT id, project_id, task_id, run_id, summary, created_at, updated_at
FROM progress
WHERE project_id = ?
ORDER BY created_at DESC, id DESC;

-- name: ListProgressByProjectAndTask :many
SELECT id, project_id, task_id, run_id, summary, created_at, updated_at
FROM progress
WHERE project_id = ? AND task_id = ?
ORDER BY created_at DESC, id DESC;

-- name: DeleteTaskStepsByTask :exec
DELETE FROM task_step
WHERE task_id = ?;

-- name: DeleteTaskStepsByProject :exec
DELETE FROM task_step
WHERE task_id IN (SELECT id FROM task WHERE project_id = ?);

-- name: DeleteTasksByProject :exec
DELETE FROM task WHERE project_id = ?;
