# 使用 MySQL

PostgreSQL 是主线数据库，MySQL 8.4 是可选的另一种数据库：同一份代码，按 `ADMIN_DATABASE_URL` 的协议头选择，不需要单独的开关。

```text
postgres://admin:admin@127.0.0.1:15432/base_admin?sslmode=disable   # PostgreSQL（默认）
mysql://admin:admin@127.0.0.1:13307/base_admin                       # MySQL
```

## 本地开发

```bash
make deps-up-mysql      # 启动 MySQL 127.0.0.1:13307 和 Redis
make migrate-up-mysql   # 在 MySQL 上执行迁移
make run-mysql          # 后端连 MySQL 启动
make test-integration-mysql   # 全部后端测试在 MySQL 上跑一遍
```

`make deps-down` 会同时停掉 MySQL（数据卷保留）。CI 里 `server` 和 `server-mysql` 两个任务分别在 PostgreSQL 和 MySQL 上跑全部后端测试。

## 目前的范围

| 功能 | PostgreSQL | MySQL |
|---|---|---|
| 登录、会话、权限、用户 / 角色 / 部门 / 岗位 / 菜单 | ✓ | ✓ |
| 字典、系统参数、基础设置、安全策略 | ✓ | ✓ |
| 通知公告、个人消息、审计日志、文件、定时任务、监控 | ✓ | ✓ |
| 代码生成器、AI 接口建表 | ✓ | ✓ |

## 部署要求

用 Docker 部署时直接用 `deploy/docker/compose.prod.mysql.yaml`（MySQL 8.4、Redis、迁移、后端、nginx），下面几条它都已经设置好。数据库密码会拼进连接地址，只用字母、数字和 `-_.~`，其他字符要做 URL 编码（`@` 写成 `%40`）。

- MySQL 8.4 及以上（用到 CHECK 约束、函数索引、表达式默认值、`INSERT … AS excluded` 行别名）。
- 数据库建议 `CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_as_cs`。迁移建的每张表都显式指定了这个排序规则，数据库默认值不同也不影响。
- 迁移会创建一个触发器（`sys_users_session_gen_monotonic`，保证会话代数只增不减）。开启 binlog 时，执行迁移的账号需要 `TRIGGER` 权限，并且要么有 `SUPER`，要么服务器设置了 `log_bin_trust_function_creators=1`。
- URL 里可以带驱动参数，例如 `?tls=true`。`parseTime`、`loc`、`charset`、`collation`、`sql_mode`、`time_zone`、`transaction_isolation`、`multiStatements`、`clientFoundRows` 由程序设置，写在 URL 里会被拒绝。

程序给每个连接设置的会话参数，用来让 MySQL 的行为和 PostgreSQL 一致：

| 设置 | 作用 |
|---|---|
| `time_zone='+00:00'`、`parseTime`、`loc=UTC` | 时间一律按 UTC 存取，对应 PostgreSQL 的 `timestamptz` |
| `transaction_isolation='READ-COMMITTED'` | PostgreSQL 的默认隔离级别；先加锁后读的逻辑依赖它 |
| `sql_mode` 含 `STRICT_ALL_TABLES`、`ANSI_QUOTES`、`PIPES_AS_CONCAT` | 非法值直接报错；`"x"` 是标识符；`\|\|` 是字符串拼接 |
| `clientFoundRows` | 受影响行数按「匹配到的行」计算，和 PostgreSQL 一样 |
| `max_execution_time` | 对应 `database.statement_timeout`（只限制 SELECT） |

## 写两边都能跑的 SQL

业务代码里手写的 SQL 只用两边共有的写法，确实没有共同写法的走 `internal/platform/database` 的函数：

| 需要 | 写法 |
|---|---|
| 当前时间 | `CURRENT_TIMESTAMP(6)`（不要用 `now()`）；时间差在 Go 里算好再传参，不写 `interval` |
| 不区分大小写的模糊查询 | `LOWER(col) LIKE LOWER(?)`（不要用 `ILIKE`） |
| 保留字做列名 | 双引号：`"key"`、`"trigger"` |
| 插入并取自增 ID | `database.InsertID(tx, "INSERT INTO t (a) VALUES (?)", a)`（不要写 `RETURNING`） |
| 存在则更新 | `"INSERT … VALUES (…) " + database.OnConflictUpdate(tx, "code") + " name = excluded.name"` |
| 存在则跳过 | `"INSERT … VALUES (…) " + database.OnConflictDoNothing(tx, "t", "col")` |
| 事务级互斥锁 | `database.LockTx(tx, "名字")` |
| 限制锁等待 | `database.SetLockTimeout(tx, d)`，必须在 `database.RunTx` 里调用 |
| 转成文本、格式化日期 | `database.CastText(db, expr)`、`database.FormatDate(db, expr)` |
| 字符串列表列 | `database.TextArray`（PostgreSQL 是 `text[]`，MySQL 是 JSON 数组） |
| 判断数据库 | `database.IsMySQL(db)`，尽量少用 |

另外几个 MySQL 的限制：

- `UPDATE` / `DELETE` 的子查询不能读同一张表，也不能在 `IN (SELECT … LIMIT ?)` 里用 `LIMIT`；包一层派生表：`WHERE id IN (SELECT id FROM (SELECT id FROM t WHERE … LIMIT ?) AS batch)`。
- 没有 `UPDATE … RETURNING`、带写操作的 `WITH`、`DISTINCT ON`、`FILTER (WHERE …)`、`generate_series`：拆成同一事务里的几条语句，或改用 `ROW_NUMBER()`、`COUNT(CASE WHEN … END)`、在 Go 里分组。
- `NOT IN` 一个空列表时一行都不匹配；列表可能为空时在 Go 里去掉这个条件。

## 代码生成器

开发库是 MySQL 时，代码生成器读 MySQL 的表结构，AI 接口建表生成 MySQL 的迁移，写到 `server/migrations/app/mysql`。

- 读表时 MySQL 类型按下表对应到生成器的字段类型；`enum`、`set`、`blob`、`time`、`year`、`bit` 等不支持，可为空或有默认值的列会被跳过，必填的会让整张表不能生成。

  | MySQL | 当作 | 控件 |
  |---|---|---|
  | `bigint`（`id`、`*_id`、`created_by`） / `int` / `smallint`、`tinyint` | 整数 / ID | 数字输入；ID 是 JSON 字符串 |
  | `tinyint(1)`（`boolean`） | 布尔 | 开关 |
  | `decimal` | 定点数，JSON 字符串 | 数字输入 |
  | `float` / `double` | 浮点 | 数字输入 |
  | `varchar` / `char` / `text` 系列 | 文本 | 输入框 / 多行文本 |
  | `datetime` / `timestamp` | 时间（UTC） | 日期时间 |
  | `date` | 日期 | 日期 |
  | `json` | JSON 文本 | 多行文本；名为 `attachments` 时是附件上传 |

- 主键要求和 PostgreSQL 一样：`id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY`。
- 生成的后端代码两种数据库都能运行：需要转换的列（`decimal`、`date`、`json`、关联名称）通过 `database.CastText` / `database.FormatDate` 读取，插入用 `database.InsertID`。只有列默认值是数据库专有的表达式（例如 `gen_random_uuid()`）时，插入语句才会按生成时的数据库写。
- AI 接口建的 MySQL 表是一条 `CREATE TABLE`，注释、索引、外键都写在里面，失败不会留下半个迁移。执行前先用临时表名（`tmp_try_…`）建一次再删除，检查 SQL 能否执行。
- 同时维护 `server/migrations/app` 和 `server/migrations/app/mysql`（两边最后一个迁移编号相同）时，AI 接口建表会把另一种数据库的迁移也写进去（不执行），项目换数据库时生成的模块照样能用。
- `text` 字段在 MySQL 上建成 `mediumtext`（16 MB）。手写的 `text` 列最多 64 KB，超出时生成的接口返回 422「内容超过字段允许的长度」，不会报 500。
- 软删除表的唯一字段用函数索引 `((CASE WHEN deleted_at IS NULL THEN col END))`，等价于 PostgreSQL 的部分唯一索引。唯一的 string 字段最长 768（MySQL 索引的上限）。

## 写 MySQL 迁移

脚手架的迁移在 `server/migrations/`（PostgreSQL）和 `server/migrations/mysql/`（MySQL），编号和文件名一一对应，单元测试会检查两边是否一致。业务迁移同理：`server/migrations/app/` 和 `server/migrations/app/mysql/`；只部署在一种数据库上的项目，只维护那一种即可。

```bash
make new-migration NAME=create_biz_orders DIALECT=mysql   # 生成 MySQL 版的建表模板
```

不给 `DIALECT` 时按 `ADMIN_DATABASE_URL` 判断。对照 PostgreSQL 的写法：

| PostgreSQL | MySQL |
|---|---|
| `bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY` | `bigint NOT NULL AUTO_INCREMENT PRIMARY KEY` |
| `timestamptz DEFAULT now()` | `datetime(6) DEFAULT CURRENT_TIMESTAMP(6)` |
| `jsonb DEFAULT '[]'`、`text DEFAULT ''` | `json DEFAULT (JSON_ARRAY())`、`text DEFAULT ('')`（只能用表达式默认值） |
| `text[]` | `json`，加 `CHECK (JSON_TYPE(col) = 'ARRAY')` |
| 列定义里的 `REFERENCES t (id)` | 表级 `CONSTRAINT x_fk FOREIGN KEY (col) REFERENCES t (id)`（MySQL 会静默忽略列定义里的 REFERENCES） |
| `COMMENT ON COLUMN t.c IS '…'` | 列定义后写 `COMMENT '…'`，表的注释写 `) … COMMENT='…'` |
| `CHECK (code ~ '^…$')`、`btrim(x)` | `CHECK (REGEXP_LIKE(code, '^…$', 'c'))`、`TRIM(x)` |
| 部分唯一索引 `… (lower(name)) WHERE deleted_at IS NULL` | 函数索引 `… ((CASE WHEN deleted_at IS NULL THEN lower(name) END))`（NULL 不参与唯一判断） |
| 其他部分索引 `… WHERE …` | 去掉 `WHERE`，把条件列放进索引 |
| `key`、`trigger` 等保留字列名 | 用反引号或双引号 |
| `INSERT … ON CONFLICT DO NOTHING` | `INSERT … ON DUPLICATE KEY UPDATE col = col` |

两点要注意：

- MySQL 不允许 CHECK 约束引用自增列，或引用带 `ON DELETE SET NULL / CASCADE` 外键的列（所以 `CHECK (id <> parent_id)` 这类约束只在 PostgreSQL 上有，由程序校验）。
- MySQL 的 `text` 最多 64 KB（PostgreSQL 不限），可能更长的列用 `mediumtext`；`varchar` 建索引最多 768 个字符（utf8mb4），所以部门路径 `path` 是 `varchar(768)`，大约支持 30 层以上的部门树。
- `max_execution_time` 只限制 SELECT，写操作没有语句超时（PostgreSQL 的 `statement_timeout` 也限制写），只受锁等待超时约束。
- MySQL 的 DDL 不能回滚：迁移执行到一半失败会留下已执行的部分，并把版本标为 dirty。一个迁移文件尽量只做一件事，down 写成 `IF EXISTS`，修复后用 `admin migrate <track> force <版本>` 恢复。
