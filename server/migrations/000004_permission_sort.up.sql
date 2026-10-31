-- Declaration order of permission codes, so UIs list them as modules
-- declare them (view first) instead of alphabetically.
ALTER TABLE sys_permissions ADD COLUMN sort integer NOT NULL DEFAULT 0;
