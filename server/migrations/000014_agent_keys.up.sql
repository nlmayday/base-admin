-- API keys of the AI agent API (开发工具 → AI 接口). A key acts as the user
-- who created it; only its SHA-256 is stored, the key itself is shown once.

CREATE TABLE sys_agent_keys (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         varchar(64) NOT NULL CHECK (btrim(name) <> ''),
    -- The first characters of the key, shown in the list to tell keys apart.
    prefix       varchar(16) NOT NULL,
    key_hash     char(64)    NOT NULL UNIQUE,
    user_id      bigint      NOT NULL REFERENCES sys_users (id) ON DELETE CASCADE,
    expires_at   timestamptz,
    last_used_at timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sys_agent_keys_user_idx ON sys_agent_keys (user_id);
