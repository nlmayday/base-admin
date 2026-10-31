-- Schedule the audit log cleanup once, so the retention days in 基础设置
-- take effect without manual setup. Empty params use that setting.
-- Administrators may edit or delete the job; it is not re-created.
INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, remark)
VALUES ('清理审计日志', 'audit.cleanup', '30 3 * * *', '{}', true,
        '每天凌晨 3:30 按「基础设置 → 日志保留天数」删除过期的登录日志与操作日志');
