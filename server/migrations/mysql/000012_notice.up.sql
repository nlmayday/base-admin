-- MySQL version of ../000012_notice.up.sql. Column comments are part of the
-- column definitions; ON CONFLICT DO NOTHING becomes a no-op
-- ON DUPLICATE KEY UPDATE.

CREATE TABLE sys_notices (
    id           bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title        varchar(128) NOT NULL COMMENT '标题' CHECK (TRIM(title) <> ''),
    -- Dictionary notice_type: 1 通知, 2 公告.
    type         smallint     NOT NULL DEFAULT 1 COMMENT '类型',
    content      text         NOT NULL COMMENT '内容' CHECK (TRIM(content) <> ''),
    -- NULL while a draft; set by publish, cleared by revoke.
    published_at datetime(6) COMMENT '发布时间',
    version      integer      NOT NULL DEFAULT 0,
    created_at   datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at   datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by   bigint,
    updated_by   bigint
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE INDEX sys_notices_published_idx ON sys_notices (published_at DESC, id DESC);

CREATE TABLE sys_notice_reads (
    notice_id bigint      NOT NULL,
    user_id   bigint      NOT NULL,
    read_at   datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id, notice_id),
    CONSTRAINT sys_notice_reads_notice_fk FOREIGN KEY (notice_id) REFERENCES sys_notices (id) ON DELETE CASCADE,
    CONSTRAINT sys_notice_reads_user_fk FOREIGN KEY (user_id) REFERENCES sys_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

-- The notice type dictionary; kept when an administrator already made one.
INSERT INTO sys_dict_types (code, name, remark)
VALUES ('notice_type', '公告类型', '通知公告的类型')
ON DUPLICATE KEY UPDATE code = code;

INSERT INTO sys_dict_items (type_id, label, value, sort, tag_type)
SELECT t.id, v.label, v.value, v.sort, v.tag_type
FROM sys_dict_types t,
     (SELECT '通知' AS label, '1' AS value, 1 AS sort, 'primary' AS tag_type
      UNION ALL SELECT '公告', '2', 2, 'warning') AS v
WHERE t.code = 'notice_type'
ON DUPLICATE KEY UPDATE sort = sys_dict_items.sort;
