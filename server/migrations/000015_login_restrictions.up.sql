-- Login restrictions in the security policy: an IP allowlist and a daily
-- login window, both skipped for the exempt roles. Empty means no limit.
ALTER TABLE sys_security_policy
    ADD COLUMN ip_allowlist       jsonb       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(ip_allowlist) = 'array'),
    ADD COLUMN login_exempt_roles jsonb       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(login_exempt_roles) = 'array'),
    -- HH:MM; both empty or both set. An end before the start spans midnight.
    ADD COLUMN login_time_start   varchar(5)  NOT NULL DEFAULT '',
    ADD COLUMN login_time_end     varchar(5)  NOT NULL DEFAULT '',
    ADD COLUMN login_time_zone    varchar(64) NOT NULL DEFAULT 'Asia/Shanghai';

COMMENT ON COLUMN sys_security_policy.ip_allowlist IS '允许登录的 IP 或网段（CIDR），空表示不限制';
COMMENT ON COLUMN sys_security_policy.login_exempt_roles IS '不受 IP 白名单和登录时段限制的角色编码';
COMMENT ON COLUMN sys_security_policy.login_time_start IS '允许登录的开始时间 HH:MM';
COMMENT ON COLUMN sys_security_policy.login_time_end IS '允许登录的结束时间 HH:MM';
COMMENT ON COLUMN sys_security_policy.login_time_zone IS '登录时段的时区';
