-- MySQL version of ../000002_permissions_menus.up.sql. text[] becomes a JSON
-- array; CHECK (id <> parent_id) is left out because MySQL forbids CHECK
-- constraints on AUTO_INCREMENT columns (the application rejects it).
CREATE TABLE sys_permissions (
    code      varchar(128) PRIMARY KEY,
    module    varchar(64)  NOT NULL,
    name      varchar(128) NOT NULL,
    menu_code varchar(128),
    requires  json         NOT NULL DEFAULT (JSON_ARRAY()) CHECK (JSON_TYPE(requires) = 'ARRAY'),
    status    varchar(16)  NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deprecated')),
    synced_at datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE TABLE sys_menus (
    id              bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code            varchar(128) NOT NULL UNIQUE,
    parent_id       bigint,
    type            varchar(8)   NOT NULL CHECK (type IN ('dir', 'menu', 'link')),
    title           varchar(64)  NOT NULL,
    path            varchar(255) NOT NULL DEFAULT '',
    component       varchar(255) NOT NULL DEFAULT '',
    icon            varchar(64)  NOT NULL DEFAULT '',
    sort            integer      NOT NULL DEFAULT 0,
    hidden          boolean      NOT NULL DEFAULT false,
    keep_alive      boolean      NOT NULL DEFAULT false,
    permission_code varchar(128),
    external_url    varchar(512) NOT NULL DEFAULT '',
    source          varchar(8)   NOT NULL CHECK (source IN ('module', 'custom')),
    module          varchar(64)  NOT NULL DEFAULT '',
    version         integer      NOT NULL DEFAULT 0,
    created_at      datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at      datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by      bigint,
    updated_by      bigint,
    CONSTRAINT sys_menus_parent_fk FOREIGN KEY (parent_id) REFERENCES sys_menus (id) ON DELETE SET NULL,
    CONSTRAINT sys_menus_permission_fk FOREIGN KEY (permission_code) REFERENCES sys_permissions (code),
    CHECK (type <> 'menu' OR permission_code IS NOT NULL)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

CREATE INDEX sys_menus_parent_id_idx ON sys_menus (parent_id);
