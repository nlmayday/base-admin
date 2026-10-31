-- Permission codes are declared in code and synchronized at startup
-- (design doc 4.4, 5.1). Removed codes become 'deprecated', never deleted.
CREATE TABLE sys_permissions (
    code      varchar(128) PRIMARY KEY,
    module    varchar(64)  NOT NULL,
    name      varchar(128) NOT NULL,
    menu_code varchar(128),
    requires  text[]       NOT NULL DEFAULT '{}',
    status    varchar(16)  NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deprecated')),
    synced_at timestamptz  NOT NULL DEFAULT now()
);

-- Menus are UI structure only. Module menus are keyed by a stable code;
-- custom menus are created by administrators.
CREATE TABLE sys_menus (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code            varchar(128) NOT NULL UNIQUE,
    parent_id       bigint REFERENCES sys_menus (id) ON DELETE SET NULL,
    type            varchar(8)   NOT NULL CHECK (type IN ('dir', 'menu', 'link')),
    title           varchar(64)  NOT NULL,
    path            varchar(255) NOT NULL DEFAULT '',
    component       varchar(255) NOT NULL DEFAULT '',
    icon            varchar(64)  NOT NULL DEFAULT '',
    sort            integer      NOT NULL DEFAULT 0,
    hidden          boolean      NOT NULL DEFAULT false,
    keep_alive      boolean      NOT NULL DEFAULT false,
    permission_code varchar(128) REFERENCES sys_permissions (code),
    external_url    varchar(512) NOT NULL DEFAULT '',
    source          varchar(8)   NOT NULL CHECK (source IN ('module', 'custom')),
    module          varchar(64)  NOT NULL DEFAULT '',
    version         integer      NOT NULL DEFAULT 0,
    created_at      timestamptz  NOT NULL DEFAULT now(),
    updated_at      timestamptz  NOT NULL DEFAULT now(),
    created_by      bigint,
    updated_by      bigint,
    CHECK (type <> 'menu' OR permission_code IS NOT NULL),
    CHECK (id <> parent_id)
);

CREATE INDEX sys_menus_parent_id_idx ON sys_menus (parent_id);
