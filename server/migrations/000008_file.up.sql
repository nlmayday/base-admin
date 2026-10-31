-- File module: media library categories and uploaded files
-- (docs/p1-p3-plan.md, file).

CREATE TABLE sys_file_categories (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       varchar(64) NOT NULL CHECK (btrim(name) <> ''),
    sort       integer     NOT NULL DEFAULT 0,
    version    integer     NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint
);
CREATE UNIQUE INDEX sys_file_categories_name_uq ON sys_file_categories (lower(name));

CREATE TABLE sys_files (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Soft-deleted files keep no claim on a category; live files block its
    -- deletion in the application (60001).
    category_id   bigint REFERENCES sys_file_categories (id) ON DELETE SET NULL,
    original_name varchar(255) NOT NULL CHECK (btrim(original_name) <> ''),
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
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);
CREATE INDEX sys_files_created_idx ON sys_files (created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX sys_files_category_idx ON sys_files (category_id) WHERE deleted_at IS NULL;
CREATE INDEX sys_files_kind_idx ON sys_files (kind) WHERE deleted_at IS NULL;
