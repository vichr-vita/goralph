-- +goose Up
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_run_one_active_per_project ON run (project_id) WHERE status = 'running';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_run_one_active_per_project;
-- +goose StatementEnd
