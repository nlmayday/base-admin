-- MySQL version of ../000013_notice_attachments.up.sql.
ALTER TABLE sys_notices ADD COLUMN attachments json NOT NULL DEFAULT (JSON_ARRAY()) COMMENT '附件'
    CHECK (JSON_TYPE(attachments) = 'ARRAY');
