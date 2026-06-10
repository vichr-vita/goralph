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

-- name: CreateTaskStep :one
INSERT INTO task_step (task_id, position, description)
VALUES (?, ?, ?)
RETURNING id, task_id, position, description, created_at, updated_at;

-- name: DeleteTaskStepsByProject :exec
DELETE FROM task_step
WHERE task_id IN (SELECT id FROM task WHERE project_id = ?);

-- name: DeleteTasksByProject :exec
DELETE FROM task WHERE project_id = ?;
