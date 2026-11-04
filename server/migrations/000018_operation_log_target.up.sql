-- The object an operation acted on (player:123, order:A1), set by the
-- handler with audit.SetTarget, so logs can be filtered by it.
ALTER TABLE sys_operation_logs
    ADD COLUMN target varchar(255) NOT NULL DEFAULT '';

CREATE INDEX sys_operation_logs_target_idx ON sys_operation_logs (target, created_at DESC);

COMMENT ON COLUMN sys_operation_logs.target IS '操作对象，如 player:123';
