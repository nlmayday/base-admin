-- Personal messages: sent by business modules to one user (core/message),
-- shown with the notices behind the header bell and in 我的消息.
CREATE TABLE sys_user_messages (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint        NOT NULL REFERENCES sys_users (id) ON DELETE CASCADE,
    -- Source, "<module>.<kind>" such as crm.reminder.
    category   varchar(64)   NOT NULL,
    title      varchar(128)  NOT NULL CHECK (btrim(title) <> ''),
    content    varchar(2000) NOT NULL DEFAULT '',
    -- Frontend route opened on click; '' for none.
    link       varchar(512)  NOT NULL DEFAULT '',
    read_at    timestamptz,
    created_at timestamptz   NOT NULL DEFAULT now()
);

COMMENT ON TABLE sys_user_messages IS '个人消息';
COMMENT ON COLUMN sys_user_messages.category IS '来源';
COMMENT ON COLUMN sys_user_messages.title IS '标题';
COMMENT ON COLUMN sys_user_messages.content IS '内容';
COMMENT ON COLUMN sys_user_messages.link IS '跳转链接';
COMMENT ON COLUMN sys_user_messages.read_at IS '已读时间';

-- The inbox lists a user's messages newest first; unread ones are counted.
CREATE INDEX sys_user_messages_user_idx ON sys_user_messages (user_id, created_at DESC, id DESC);
CREATE INDEX sys_user_messages_unread_idx ON sys_user_messages (user_id) WHERE read_at IS NULL;
-- The cleanup job deletes old read messages.
CREATE INDEX sys_user_messages_read_idx ON sys_user_messages (read_at) WHERE read_at IS NOT NULL;

-- Scheduled once; administrators may edit or delete the job.
INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, remark)
VALUES ('清理个人消息', 'notice.message-cleanup', '40 3 * * *', '{}', true,
        '每天凌晨 3:40 删除已读超过 90 天的个人消息，以及已删除用户的消息；参数 {"days": N} 可修改天数');
