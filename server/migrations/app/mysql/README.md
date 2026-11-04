# 业务迁移（MySQL）

上一级目录 `app/` 的 MySQL 版本：数据库地址是 `mysql://` 时，`admin migrate up` 执行这里的迁移。

- 编号、文件名与 `app/` 下的 PostgreSQL 迁移一一对应；只部署在一种数据库上的项目，只维护那一种即可。
- 表统一使用 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs`（区分大小写，与 PostgreSQL 一致）。
- 写法对照见 `docs/mysql.md`。
