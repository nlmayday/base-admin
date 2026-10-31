-- System configuration: data dictionary, config params with change
-- history, and the built-in settings stored as configs (docs/p1-p3-plan.md).

CREATE TABLE sys_dict_types (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       varchar(64)  NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{1,63}$'),
    name       varchar(64)  NOT NULL CHECK (btrim(name) <> ''),
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint,
    CONSTRAINT sys_dict_types_code_uq UNIQUE (code)
);

CREATE TABLE sys_dict_items (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type_id    bigint       NOT NULL REFERENCES sys_dict_types (id) ON DELETE CASCADE,
    label      varchar(64)  NOT NULL CHECK (btrim(label) <> ''),
    value      varchar(64)  NOT NULL CHECK (btrim(value) <> ''),
    sort       integer      NOT NULL DEFAULT 0,
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    -- Element Plus el-tag type; '' means a plain tag.
    tag_type   varchar(16)  NOT NULL DEFAULT ''
        CHECK (tag_type IN ('', 'primary', 'success', 'info', 'warning', 'danger')),
    remark     varchar(255) NOT NULL DEFAULT '',
    version    integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint,
    CONSTRAINT sys_dict_items_type_value_uq UNIQUE (type_id, value)
);
CREATE INDEX sys_dict_items_type_sort_idx ON sys_dict_items (type_id, sort, id);

CREATE TABLE sys_configs (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key         varchar(128) NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
    value       text         NOT NULL DEFAULT '',
    value_type  varchar(16)  NOT NULL DEFAULT 'string'
        CHECK (value_type IN ('string', 'number', 'boolean', 'json')),
    group_name  varchar(64)  NOT NULL DEFAULT 'default' CHECK (btrim(group_name) <> ''),
    name        varchar(64)  NOT NULL CHECK (btrim(name) <> ''),
    description varchar(255) NOT NULL DEFAULT '',
    -- editable = false: read-only through the config API.
    editable    boolean      NOT NULL DEFAULT true,
    -- builtin: seeded by a module; managed through typed settings only.
    builtin     boolean      NOT NULL DEFAULT false,
    version     integer      NOT NULL DEFAULT 0,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now(),
    created_by  bigint,
    updated_by  bigint,
    CONSTRAINT sys_configs_key_uq UNIQUE (key)
);
CREATE INDEX sys_configs_group_idx ON sys_configs (group_name, key);

CREATE TABLE sys_config_changes (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    config_id  bigint       NOT NULL REFERENCES sys_configs (id) ON DELETE CASCADE,
    key        varchar(128) NOT NULL,
    -- NULL old_value marks the creation of the config.
    old_value  text,
    new_value  text         NOT NULL,
    changed_by bigint REFERENCES sys_users (id) ON DELETE SET NULL,
    changed_at timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX sys_config_changes_config_idx ON sys_config_changes (config_id, changed_at DESC, id DESC);
