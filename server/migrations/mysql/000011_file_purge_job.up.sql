-- Schedule the purge of soft-deleted files once (see 000010 for why built-in
-- jobs come from migrations). Administrators may edit or delete it.
INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, remark)
VALUES ('清理已删除文件', 'file.purge-deleted', '15 * * * *', '{}', true,
        '每小时清除已删除文件残留的存储对象（删除时存储暂不可用等情况）');
