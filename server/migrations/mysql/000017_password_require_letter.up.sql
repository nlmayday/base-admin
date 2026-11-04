-- MySQL version of ../000017_password_require_letter.up.sql.
ALTER TABLE sys_security_policy
    ADD COLUMN password_require_letter boolean NOT NULL DEFAULT false COMMENT '密码须含字母（大小写不限）'
    AFTER password_require_lower;
