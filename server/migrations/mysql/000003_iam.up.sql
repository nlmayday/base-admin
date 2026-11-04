-- MySQL version of ../000003_iam.up.sql. Partial indexes become plain or
-- functional indexes: a NULL key never conflicts in a unique index, so
-- (CASE WHEN deleted_at IS NULL THEN lower(username) END) is unique among
-- live users only, like the partial index on PostgreSQL.

CREATE TABLE sys_departments (
    id             bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    parent_id      bigint,
    -- Materialized path such as '/1/5/12/', maintained by the application.
    path           varchar(768) NOT NULL DEFAULT '',
    name           varchar(64)  NOT NULL,
    leader_user_id bigint,
    sort           integer      NOT NULL DEFAULT 0,
    status         smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    version        integer      NOT NULL DEFAULT 0,
    created_at     datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at     datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by     bigint,
    updated_by     bigint,
    CONSTRAINT sys_departments_parent_fk FOREIGN KEY (parent_id) REFERENCES sys_departments (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_departments_path_idx ON sys_departments (path);
CREATE INDEX sys_departments_parent_id_idx ON sys_departments (parent_id);

CREATE TABLE sys_users (
    id                   bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username             varchar(32)  NOT NULL,
    password_hash        text         NOT NULL,
    nickname             varchar(64)  NOT NULL DEFAULT '',
    email                varchar(128) NOT NULL DEFAULT '',
    phone                varchar(32)  NOT NULL DEFAULT '',
    avatar_url           varchar(512) NOT NULL DEFAULT '',
    dept_id              bigint,
    status               smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    -- Authoritative session generation (design doc 6.5). Only ever grows.
    session_gen          bigint       NOT NULL DEFAULT 1 CHECK (session_gen >= 1),
    must_change_password boolean      NOT NULL DEFAULT false,
    password_changed_at  datetime(6),
    last_login_at        datetime(6),
    last_login_ip        varchar(64)  NOT NULL DEFAULT '',
    version              integer      NOT NULL DEFAULT 0,
    created_at           datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at           datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by           bigint,
    updated_by           bigint,
    deleted_at           datetime(6),
    CONSTRAINT sys_users_dept_fk FOREIGN KEY (dept_id) REFERENCES sys_departments (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE UNIQUE INDEX sys_users_username_uq ON sys_users ((CASE WHEN deleted_at IS NULL THEN lower(username) END));
CREATE INDEX sys_users_dept_id_idx ON sys_users (dept_id);

-- Invariant I3 (the session generation never decreases) is kept by the
-- application on MySQL: the only writer sets GREATEST(session_gen, ?) under
-- the row lock (auth/revoke.go). PostgreSQL also has a trigger; creating one
-- here would need SUPER (or log_bin_trust_function_creators) with binary
-- logging on, which managed databases rarely grant.

ALTER TABLE sys_departments
    ADD CONSTRAINT sys_departments_leader_fk FOREIGN KEY (leader_user_id) REFERENCES sys_users (id) ON DELETE SET NULL;

CREATE TABLE sys_roles (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code       varchar(32)  NOT NULL UNIQUE,
    name       varchar(64)  NOT NULL,
    data_scope varchar(24)  NOT NULL DEFAULT 'self'
        CHECK (data_scope IN ('all', 'dept_and_children', 'dept', 'self', 'custom')),
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    is_builtin boolean      NOT NULL DEFAULT false,
    sort       integer      NOT NULL DEFAULT 0,
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by bigint,
    updated_by bigint
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_role_depts (
    role_id bigint NOT NULL,
    dept_id bigint NOT NULL,
    PRIMARY KEY (role_id, dept_id),
    CONSTRAINT sys_role_depts_role_fk FOREIGN KEY (role_id) REFERENCES sys_roles (id) ON DELETE CASCADE,
    CONSTRAINT sys_role_depts_dept_fk FOREIGN KEY (dept_id) REFERENCES sys_departments (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_user_roles (
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT sys_user_roles_user_fk FOREIGN KEY (user_id) REFERENCES sys_users (id) ON DELETE CASCADE,
    CONSTRAINT sys_user_roles_role_fk FOREIGN KEY (role_id) REFERENCES sys_roles (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_user_roles_role_id_idx ON sys_user_roles (role_id);

-- The single source of truth for authorization (design doc 5.1).
CREATE TABLE sys_role_permissions (
    role_id         bigint       NOT NULL,
    permission_code varchar(128) NOT NULL,
    PRIMARY KEY (role_id, permission_code),
    CONSTRAINT sys_role_permissions_role_fk FOREIGN KEY (role_id) REFERENCES sys_roles (id) ON DELETE CASCADE,
    CONSTRAINT sys_role_permissions_code_fk FOREIGN KEY (permission_code) REFERENCES sys_permissions (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_positions (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code       varchar(32)  NOT NULL UNIQUE,
    name       varchar(64)  NOT NULL,
    sort       integer      NOT NULL DEFAULT 0,
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by bigint,
    updated_by bigint
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_user_positions (
    user_id     bigint NOT NULL,
    position_id bigint NOT NULL,
    PRIMARY KEY (user_id, position_id),
    CONSTRAINT sys_user_positions_user_fk FOREIGN KEY (user_id) REFERENCES sys_users (id) ON DELETE CASCADE,
    CONSTRAINT sys_user_positions_position_fk FOREIGN KEY (position_id) REFERENCES sys_positions (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_user_positions_position_id_idx ON sys_user_positions (position_id);
