DELETE FROM sys_jobs WHERE handler_key = 'notice.message-cleanup';
DROP TABLE sys_user_messages;
