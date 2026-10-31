DELETE FROM sys_jobs WHERE handler_key = 'file.purge-deleted' AND name = '清理已删除文件';
