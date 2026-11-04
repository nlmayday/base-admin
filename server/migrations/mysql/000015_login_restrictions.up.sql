-- MySQL version of ../000015_login_restrictions.up.sql.
ALTER TABLE sys_security_policy
    ADD COLUMN ip_allowlist       json        NOT NULL DEFAULT (JSON_ARRAY())
        COMMENT '允许登录的 IP 或网段（CIDR），空表示不限制' CHECK (JSON_TYPE(ip_allowlist) = 'ARRAY'),
    ADD COLUMN login_exempt_roles json        NOT NULL DEFAULT (JSON_ARRAY())
        COMMENT '不受 IP 白名单和登录时段限制的角色编码' CHECK (JSON_TYPE(login_exempt_roles) = 'ARRAY'),
    -- HH:MM; both empty or both set. An end before the start spans midnight.
    ADD COLUMN login_time_start   varchar(5)  NOT NULL DEFAULT '' COMMENT '允许登录的开始时间 HH:MM',
    ADD COLUMN login_time_end     varchar(5)  NOT NULL DEFAULT '' COMMENT '允许登录的结束时间 HH:MM',
    ADD COLUMN login_time_zone    varchar(64) NOT NULL DEFAULT 'Asia/Shanghai' COMMENT '登录时段的时区';
