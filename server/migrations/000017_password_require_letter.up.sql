-- A password rule "contains a letter, either case" (passwords of at least
-- N characters with letters and digits), besides upper and lower case.
ALTER TABLE sys_security_policy
    ADD COLUMN password_require_letter boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN sys_security_policy.password_require_letter IS '密码须含字母（大小写不限）';
