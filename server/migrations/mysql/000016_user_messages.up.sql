-- MySQL version of ../000016_user_messages.up.sql.
CREATE TABLE sys_user_messages (
    id         bigint        NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id    bigint        NOT NULL,
    -- Source, "<module>.<kind>" such as crm.reminder.
    category   varchar(64)   NOT NULL COMMENT '来源',
    title      varchar(128)  NOT NULL COMMENT '标题' CHECK (TRIM(title) <> ''),
    content    varchar(2000) NOT NULL DEFAULT '' COMMENT '内容',
    -- Frontend route opened on click; '' for none.
    link       varchar(512)  NOT NULL DEFAULT '' COMMENT '跳转链接',
    read_at    datetime(6) COMMENT '已读时间',
    created_at datetime(6)   NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT sys_user_messages_user_fk FOREIGN KEY (user_id) REFERENCES sys_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs COMMENT='个人消息';

-- The inbox lists a user's messages newest first; unread ones are counted.
CREATE INDEX sys_user_messages_user_idx ON sys_user_messages (user_id, created_at DESC, id DESC);
CREATE INDEX sys_user_messages_unread_idx ON sys_user_messages (user_id, read_at);
-- The cleanup job deletes old read messages.
CREATE INDEX sys_user_messages_read_idx ON sys_user_messages (read_at);

-- Scheduled once; administrators may edit or delete the job.
INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, remark)
VALUES ('清理个人消息', 'notice.message-cleanup', '40 3 * * *', '{}', true,
        '每天凌晨 3:40 删除已读超过 90 天的个人消息，以及已删除用户的消息；参数 {"days": N} 可修改天数');
