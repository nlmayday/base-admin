-- MySQL version of ../000004_permission_sort.up.sql.
ALTER TABLE sys_permissions ADD COLUMN sort integer NOT NULL DEFAULT 0;
