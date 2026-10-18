-- +goose Up
CREATE INDEX appointments_instance_created ON appointments (instance_id, created_at);

-- +goose Down
DROP INDEX appointments_instance_created;
