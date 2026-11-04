DROP TABLE IF EXISTS sys_user_positions;
DROP TABLE IF EXISTS sys_positions;
DROP TABLE IF EXISTS sys_role_permissions;
DROP TABLE IF EXISTS sys_user_roles;
DROP TABLE IF EXISTS sys_role_depts;
DROP TABLE IF EXISTS sys_roles;
ALTER TABLE sys_departments DROP FOREIGN KEY sys_departments_leader_fk;
DROP TABLE IF EXISTS sys_users;
DROP TABLE IF EXISTS sys_departments;
