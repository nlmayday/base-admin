-- MySQL version of ../000007_auth_security.up.sql.
CREATE TABLE sys_security_policy (
    id                      smallint    PRIMARY KEY CHECK (id = 1),
    password_min_length     integer     NOT NULL DEFAULT 8 CHECK (password_min_length BETWEEN 8 AND 72),
    password_require_upper  boolean     NOT NULL DEFAULT false,
    password_require_lower  boolean     NOT NULL DEFAULT false,
    password_require_digit  boolean     NOT NULL DEFAULT true,
    password_require_symbol boolean     NOT NULL DEFAULT false,
    captcha_mode            varchar(16) NOT NULL DEFAULT 'after_failures'
        CHECK (captcha_mode IN ('off', 'after_failures', 'always')),
    captcha_after_failures  integer     NOT NULL DEFAULT 3 CHECK (captcha_after_failures BETWEEN 1 AND 20),
    lockout_threshold       integer     NOT NULL DEFAULT 5 CHECK (lockout_threshold BETWEEN 0 AND 100),
    lockout_minutes         integer     NOT NULL DEFAULT 15 CHECK (lockout_minutes BETWEEN 1 AND 1440),
    version                 integer     NOT NULL DEFAULT 0,
    updated_at              datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_by              bigint
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

INSERT INTO sys_security_policy (id) VALUES (1);
