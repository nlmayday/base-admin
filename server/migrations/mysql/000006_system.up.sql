-- MySQL version of ../000006_system.up.sql. The PostgreSQL regex operator ~
-- becomes REGEXP_LIKE with 'c' (case-sensitive) and btrim becomes TRIM.
-- Config values are mediumtext: MySQL text holds only 64 KB, PostgreSQL text
-- has no such limit.

CREATE TABLE sys_dict_types (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code       varchar(64)  NOT NULL CHECK (REGEXP_LIKE(code, '^[a-z][a-z0-9_]{1,63}$', 'c')),
    name       varchar(64)  NOT NULL CHECK (TRIM(name) <> ''),
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by bigint,
    updated_by bigint,
    CONSTRAINT sys_dict_types_code_uq UNIQUE (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_dict_items (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    type_id    bigint       NOT NULL,
    label      varchar(64)  NOT NULL CHECK (TRIM(label) <> ''),
    value      varchar(64)  NOT NULL CHECK (TRIM(value) <> ''),
    sort       integer      NOT NULL DEFAULT 0,
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    -- Element Plus el-tag type; '' means a plain tag.
    tag_type   varchar(16)  NOT NULL DEFAULT ''
        CHECK (tag_type IN ('', 'primary', 'success', 'info', 'warning', 'danger')),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by bigint,
    updated_by bigint,
    CONSTRAINT sys_dict_items_type_fk FOREIGN KEY (type_id) REFERENCES sys_dict_types (id) ON DELETE CASCADE,
    CONSTRAINT sys_dict_items_type_value_uq UNIQUE (type_id, value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_dict_items_type_sort_idx ON sys_dict_items (type_id, sort, id);

-- key is a reserved word in MySQL; the application's SQL quotes it.
CREATE TABLE sys_configs (
    id          bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `key`       varchar(128) NOT NULL CHECK (REGEXP_LIKE(`key`, '^[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)*$', 'c')),
    value       mediumtext   NOT NULL DEFAULT (''),
    value_type  varchar(16)  NOT NULL DEFAULT 'string'
        CHECK (value_type IN ('string', 'number', 'boolean', 'json')),
    group_name  varchar(64)  NOT NULL DEFAULT 'default' CHECK (TRIM(group_name) <> ''),
    name        varchar(64)  NOT NULL CHECK (TRIM(name) <> ''),
    description varchar(255) NOT NULL DEFAULT '',
    -- editable = false: read-only through the config API.
    editable    boolean      NOT NULL DEFAULT true,
    -- builtin: seeded by a module; managed through typed settings only.
    builtin     boolean      NOT NULL DEFAULT false,
    version     integer      NOT NULL DEFAULT 0,
    created_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at  datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by  bigint,
    updated_by  bigint,
    CONSTRAINT sys_configs_key_uq UNIQUE (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_configs_group_idx ON sys_configs (group_name, `key`);

CREATE TABLE sys_config_changes (
    id         bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    config_id  bigint       NOT NULL,
    `key`      varchar(128) NOT NULL,
    -- NULL old_value marks the creation of the config.
    old_value  mediumtext,
    new_value  mediumtext   NOT NULL,
    changed_by bigint,
    changed_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT sys_config_changes_config_fk FOREIGN KEY (config_id) REFERENCES sys_configs (id) ON DELETE CASCADE,
    CONSTRAINT sys_config_changes_user_fk FOREIGN KEY (changed_by) REFERENCES sys_users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_config_changes_config_idx ON sys_config_changes (config_id, changed_at DESC, id DESC);
