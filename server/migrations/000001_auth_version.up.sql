-- Global auth version: source of truth for auth snapshot invalidation
-- (design doc 5.6). Exactly one row, incremented inside the transaction of
-- every change that affects authorization.
CREATE TABLE sys_auth_version (
    id      smallint PRIMARY KEY CHECK (id = 1),
    version bigint   NOT NULL DEFAULT 1 CHECK (version >= 1)
);

INSERT INTO sys_auth_version (id, version) VALUES (1, 1);
