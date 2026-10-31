-- IAM schema (design doc 7, 8.2).

CREATE TABLE sys_departments (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id      bigint REFERENCES sys_departments (id),
    -- Materialized path such as '/1/5/12/', maintained by the application.
    path           text         NOT NULL DEFAULT '',
    name           varchar(64)  NOT NULL,
    leader_user_id bigint,
    sort           integer      NOT NULL DEFAULT 0,
    status         smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    version        integer      NOT NULL DEFAULT 0,
    created_at     timestamptz  NOT NULL DEFAULT now(),
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    created_by     bigint,
    updated_by     bigint,
    CHECK (id <> parent_id)
);
CREATE INDEX sys_departments_path_idx ON sys_departments (path text_pattern_ops);
CREATE INDEX sys_departments_parent_id_idx ON sys_departments (parent_id);

CREATE TABLE sys_users (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username             varchar(32)  NOT NULL,
    password_hash        text         NOT NULL,
    nickname             varchar(64)  NOT NULL DEFAULT '',
    email                varchar(128) NOT NULL DEFAULT '',
    phone                varchar(32)  NOT NULL DEFAULT '',
    avatar_url           varchar(512) NOT NULL DEFAULT '',
    dept_id              bigint REFERENCES sys_departments (id),
    status               smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    -- Authoritative session generation (design doc 6.5). Only ever grows.
    session_gen          bigint       NOT NULL DEFAULT 1 CHECK (session_gen >= 1),
    must_change_password boolean      NOT NULL DEFAULT false,
    password_changed_at  timestamptz,
    last_login_at        timestamptz,
    last_login_ip        varchar(64)  NOT NULL DEFAULT '',
    version              integer      NOT NULL DEFAULT 0,
    created_at           timestamptz  NOT NULL DEFAULT now(),
    updated_at           timestamptz  NOT NULL DEFAULT now(),
    created_by           bigint,
    updated_by           bigint,
    deleted_at           timestamptz
);
CREATE UNIQUE INDEX sys_users_username_uq ON sys_users (lower(username)) WHERE deleted_at IS NULL;
CREATE INDEX sys_users_dept_id_idx ON sys_users (dept_id) WHERE deleted_at IS NULL;

-- Invariant I3 enforced in the database: the session generation never
-- decreases, whatever the application does.
CREATE FUNCTION sys_users_session_gen_monotonic() RETURNS trigger AS $$
BEGIN
    IF NEW.session_gen < OLD.session_gen THEN
        RAISE EXCEPTION 'session_gen must not decrease (% -> %)', OLD.session_gen, NEW.session_gen;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER sys_users_session_gen_monotonic
    BEFORE UPDATE OF session_gen ON sys_users
    FOR EACH ROW EXECUTE FUNCTION sys_users_session_gen_monotonic();

ALTER TABLE sys_departments
    ADD CONSTRAINT sys_departments_leader_fk FOREIGN KEY (leader_user_id) REFERENCES sys_users (id) ON DELETE SET NULL;

CREATE TABLE sys_roles (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       varchar(32)  NOT NULL UNIQUE,
    name       varchar(64)  NOT NULL,
    data_scope varchar(24)  NOT NULL DEFAULT 'self'
        CHECK (data_scope IN ('all', 'dept_and_children', 'dept', 'self', 'custom')),
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    is_builtin boolean      NOT NULL DEFAULT false,
    sort       integer      NOT NULL DEFAULT 0,
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint
);

CREATE TABLE sys_role_depts (
    role_id bigint NOT NULL REFERENCES sys_roles (id) ON DELETE CASCADE,
    dept_id bigint NOT NULL REFERENCES sys_departments (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, dept_id)
);

CREATE TABLE sys_user_roles (
    user_id bigint NOT NULL REFERENCES sys_users (id) ON DELETE CASCADE,
    role_id bigint NOT NULL REFERENCES sys_roles (id),
    PRIMARY KEY (user_id, role_id)
);
CREATE INDEX sys_user_roles_role_id_idx ON sys_user_roles (role_id);

-- The single source of truth for authorization (design doc 5.1).
CREATE TABLE sys_role_permissions (
    role_id         bigint       NOT NULL REFERENCES sys_roles (id) ON DELETE CASCADE,
    permission_code varchar(128) NOT NULL REFERENCES sys_permissions (code),
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE sys_positions (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       varchar(32)  NOT NULL UNIQUE,
    name       varchar(64)  NOT NULL,
    sort       integer      NOT NULL DEFAULT 0,
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint
);

CREATE TABLE sys_user_positions (
    user_id     bigint NOT NULL REFERENCES sys_users (id) ON DELETE CASCADE,
    position_id bigint NOT NULL REFERENCES sys_positions (id),
    PRIMARY KEY (user_id, position_id)
);
CREATE INDEX sys_user_positions_position_id_idx ON sys_user_positions (position_id);
