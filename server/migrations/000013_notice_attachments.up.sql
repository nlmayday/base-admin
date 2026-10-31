-- Notice attachments: [{name, url, size}] from the file module's attachment
-- upload (validated by internal/core/attachment).
ALTER TABLE sys_notices ADD COLUMN attachments jsonb NOT NULL DEFAULT '[]'
    CHECK (jsonb_typeof(attachments) = 'array');
COMMENT ON COLUMN sys_notices.attachments IS '附件';
