# 业务迁移

这里放业务模块的迁移，和上一级目录的脚手架迁移分开编号：

- 编号从 000001 开始，与脚手架迁移互不影响，合并上游时不会冲突。
- 版本记录在 `app_schema_migrations` 表；脚手架迁移记录在 `schema_migrations`。
- `make new-migration NAME=create_<表名>` 默认在这里建文件。
- `admin migrate up` 先执行脚手架迁移，再执行这里的迁移。

业务表可以引用脚手架的表（如 `sys_users`、`sys_departments`）；
脚手架迁移不会依赖这里的表。
