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
