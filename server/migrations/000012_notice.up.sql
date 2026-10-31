-- Notices: announcements written by administrators and shown to every user
-- (header bell and 我的消息), with a per-user read mark.

CREATE TABLE sys_notices (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title        varchar(128) NOT NULL CHECK (btrim(title) <> ''),
    -- Dictionary notice_type: 1 通知, 2 公告.
    type         smallint     NOT NULL DEFAULT 1,
    content      text         NOT NULL CHECK (btrim(content) <> ''),
    -- NULL while a draft; set by publish, cleared by revoke.
    published_at timestamptz,
    version      integer      NOT NULL DEFAULT 0,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    created_by   bigint,
    updated_by   bigint
);

COMMENT ON COLUMN sys_notices.title IS '标题';
COMMENT ON COLUMN sys_notices.type IS '类型';
COMMENT ON COLUMN sys_notices.content IS '内容';
COMMENT ON COLUMN sys_notices.published_at IS '发布时间';

CREATE INDEX sys_notices_published_idx ON sys_notices (published_at DESC, id DESC)
    WHERE published_at IS NOT NULL;

CREATE TABLE sys_notice_reads (
    notice_id bigint      NOT NULL REFERENCES sys_notices (id) ON DELETE CASCADE,
    user_id   bigint      NOT NULL REFERENCES sys_users (id) ON DELETE CASCADE,
    read_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, notice_id)
);

-- The notice type dictionary; kept when an administrator already made one.
INSERT INTO sys_dict_types (code, name, remark)
VALUES ('notice_type', '公告类型', '通知公告的类型')
ON CONFLICT (code) DO NOTHING;

INSERT INTO sys_dict_items (type_id, label, value, sort, tag_type)
SELECT t.id, v.label, v.value, v.sort, v.tag_type
FROM sys_dict_types t,
     (VALUES ('通知', '1', 1, 'primary'), ('公告', '2', 2, 'warning')) AS v (label, value, sort, tag_type)
WHERE t.code = 'notice_type'
ON CONFLICT (type_id, value) DO NOTHING;
