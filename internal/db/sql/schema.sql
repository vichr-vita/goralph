CREATE TABLE project (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE task (
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

CREATE INDEX idx_task_project_id ON task (project_id);
CREATE INDEX idx_task_status ON task (status);

CREATE TABLE task_step (
    id INTEGER PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES task (id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    description TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (task_id, position)
);

CREATE INDEX idx_task_step_task_id ON task_step (task_id);

CREATE TABLE run (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    task_id INTEGER REFERENCES task (id) ON DELETE SET NULL,
    runner_name TEXT NOT NULL,
    runner_version TEXT NOT NULL DEFAULT '',
    runner_model TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    session_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (
        status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')
    ),
    exit_code INTEGER,
    exit_signal TEXT,
    exit_error TEXT,
    pid INTEGER,
    host TEXT NOT NULL DEFAULT '',
    heartbeat_at TEXT,
    started_at TEXT,
    finished_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_run_project_id ON run (project_id);
CREATE INDEX idx_run_task_id ON run (task_id);
CREATE INDEX idx_run_status ON run (status);

CREATE TABLE progress (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    task_id INTEGER REFERENCES task (id) ON DELETE SET NULL,
    run_id INTEGER REFERENCES run (id) ON DELETE SET NULL,
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_progress_project_id ON progress (project_id);
CREATE INDEX idx_progress_task_id ON progress (task_id);
CREATE INDEX idx_progress_run_id ON progress (run_id);

CREATE TABLE feedback_command (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES project (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (project_id, name)
);

CREATE INDEX idx_feedback_command_project_id ON feedback_command (project_id);
