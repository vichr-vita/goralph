-- +goose NO TRANSACTION

-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;

CREATE TABLE task_new (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN ('pending', 'in_progress', 'blocked', 'passed', 'failed')
    ),
    progress_report TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO task_new (
    id,
    project_id,
    category,
    description,
    status,
    progress_report,
    created_at,
    updated_at
)
SELECT
    id,
    project_id,
    category,
    description,
    CASE status
        WHEN 'completed' THEN 'passed'
        WHEN 'cancelled' THEN 'failed'
        ELSE status
    END,
    progress_report,
    created_at,
    updated_at
FROM task;

DROP TABLE task;
ALTER TABLE task_new RENAME TO task;

CREATE INDEX idx_task_project_id ON task (project_id);
CREATE INDEX idx_task_status ON task (status);

PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;

CREATE TABLE task_old (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN ('pending', 'in_progress', 'completed', 'failed', 'cancelled')
    ),
    progress_report TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO task_old (
    id,
    project_id,
    category,
    description,
    status,
    progress_report,
    created_at,
    updated_at
)
SELECT
    id,
    project_id,
    category,
    description,
    CASE status
        WHEN 'passed' THEN 'completed'
        WHEN 'blocked' THEN 'cancelled'
        ELSE status
    END,
    progress_report,
    created_at,
    updated_at
FROM task;

DROP TABLE task;
ALTER TABLE task_old RENAME TO task;

CREATE INDEX idx_task_project_id ON task (project_id);
CREATE INDEX idx_task_status ON task (status);

PRAGMA foreign_keys = ON;
-- +goose StatementEnd
