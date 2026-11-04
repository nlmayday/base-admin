DELETE FROM sys_jobs WHERE handler_key = 'audit.cleanup' AND name = '清理审计日志';
