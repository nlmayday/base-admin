-- MySQL version of ../000018_operation_log_target.up.sql. One statement, so
-- it is applied entirely or not at all.
ALTER TABLE sys_operation_logs
    ADD COLUMN target varchar(255) NOT NULL DEFAULT '' COMMENT '操作对象，如 player:123' AFTER summary,
    ADD INDEX sys_operation_logs_target_idx (target, created_at DESC);
