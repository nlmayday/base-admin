-- MySQL version of ../000008_file.up.sql. The partial indexes on live files
-- become plain indexes.

CREATE TABLE sys_file_categories (
    id         bigint      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name       varchar(64) NOT NULL CHECK (TRIM(name) <> ''),
    sort       integer     NOT NULL DEFAULT 0,
    version    integer     NOT NULL DEFAULT 0,
    created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by bigint,
    updated_by bigint
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE UNIQUE INDEX sys_file_categories_name_uq ON sys_file_categories ((lower(name)));

CREATE TABLE sys_files (
    id            bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    category_id   bigint,
    original_name varchar(255) NOT NULL CHECK (TRIM(original_name) <> ''),
    -- Server-generated "yyyy/mm/dd/<random>.<ext>"; never user input.
    object_key    varchar(255) NOT NULL UNIQUE,
    storage       varchar(16)  NOT NULL,
    size          bigint       NOT NULL CHECK (size >= 0),
    mime_type     varchar(128) NOT NULL,
    ext           varchar(16)  NOT NULL,
    kind          varchar(16)  NOT NULL CHECK (kind IN ('image', 'document', 'media', 'archive', 'other')),
    sha256        char(64)     NOT NULL,
    uploaded_by   bigint,
    version       integer      NOT NULL DEFAULT 0,
    created_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at    datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    deleted_at    datetime(6),
    CONSTRAINT sys_files_category_fk FOREIGN KEY (category_id) REFERENCES sys_file_categories (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_files_created_idx ON sys_files (created_at DESC, id DESC);
CREATE INDEX sys_files_category_idx ON sys_files (category_id);
CREATE INDEX sys_files_kind_idx ON sys_files (kind);
