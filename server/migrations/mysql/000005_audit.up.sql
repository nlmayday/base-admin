-- MySQL version of ../000005_audit.up.sql.

CREATE TABLE sys_login_logs (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    kind       varchar(16)  NOT NULL CHECK (kind IN ('login', 'logout')),
    username   varchar(64)  NOT NULL DEFAULT '',
    user_id    bigint,
    success    boolean      NOT NULL,
    reason     varchar(32)  NOT NULL DEFAULT '',
    ip         varchar(64)  NOT NULL DEFAULT '',
    user_agent varchar(512) NOT NULL DEFAULT '',
    request_id varchar(64)  NOT NULL DEFAULT '',
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_login_logs_created_at_idx ON sys_login_logs (created_at DESC);
CREATE INDEX sys_login_logs_user_id_idx ON sys_login_logs (user_id, created_at DESC);
CREATE INDEX sys_login_logs_username_idx ON sys_login_logs ((lower(username)), created_at DESC);

CREATE TABLE sys_operation_logs (
    id            bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id       bigint,
    module        varchar(32)  NOT NULL DEFAULT '',
    method        varchar(8)   NOT NULL,
    route         varchar(255) NOT NULL DEFAULT '',
    path          varchar(512) NOT NULL DEFAULT '',
    summary       varchar(128) NOT NULL DEFAULT '',
    status        integer      NOT NULL DEFAULT 0,
    code          integer      NOT NULL DEFAULT 0,
    success       boolean      NOT NULL,
    duration_ms   integer      NOT NULL DEFAULT 0,
    ip            varchar(64)  NOT NULL DEFAULT '',
    user_agent    varchar(512) NOT NULL DEFAULT '',
    request_id    varchar(64)  NOT NULL DEFAULT '',
    -- Redacted JSON request body, at most 4 KB; empty when not captured.
    request_body  text         NOT NULL DEFAULT (''),
    error_message varchar(512) NOT NULL DEFAULT '',
    created_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_operation_logs_created_at_idx ON sys_operation_logs (created_at DESC);
CREATE INDEX sys_operation_logs_user_id_idx ON sys_operation_logs (user_id, created_at DESC);
CREATE INDEX sys_operation_logs_module_idx ON sys_operation_logs (module, created_at DESC);
