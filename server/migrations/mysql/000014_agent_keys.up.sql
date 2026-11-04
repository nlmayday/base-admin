-- MySQL version of ../000014_agent_keys.up.sql.

CREATE TABLE sys_agent_keys (
    id           bigint      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         varchar(64) NOT NULL CHECK (TRIM(name) <> ''),
    -- The first characters of the key, shown in the list to tell keys apart.
    prefix       varchar(16) NOT NULL,
    key_hash     char(64)    NOT NULL UNIQUE,
    user_id      bigint      NOT NULL,
    expires_at   datetime(6),
    last_used_at datetime(6),
    revoked_at   datetime(6),
    created_at   datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT sys_agent_keys_user_fk FOREIGN KEY (user_id) REFERENCES sys_users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE INDEX sys_agent_keys_user_idx ON sys_agent_keys (user_id);
