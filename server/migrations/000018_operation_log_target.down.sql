DROP INDEX IF EXISTS sys_operation_logs_target_idx;
ALTER TABLE sys_operation_logs DROP COLUMN IF EXISTS target;
