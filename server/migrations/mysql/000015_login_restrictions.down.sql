ALTER TABLE sys_security_policy
    DROP COLUMN ip_allowlist,
    DROP COLUMN login_exempt_roles,
    DROP COLUMN login_time_start,
    DROP COLUMN login_time_end,
    DROP COLUMN login_time_zone;
