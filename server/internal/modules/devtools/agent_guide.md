# Base Admin AI 接口使用说明

本接口供 AI 编程助手调用：按框架约定建业务表，再一键生成该表的增删改查
（Go 后端模块、Vue 页面、菜单、权限、接口文档）。所有生成物都遵守项目的
建表规范与代码约定，**不要手写建表 SQL 或手写 CRUD 模块**，用本接口生成，
生成后再按需修改业务逻辑。

- 基础地址：`{{BASE}}`（下文路径均相对于它）
- 鉴权：每个请求带请求头 `Authorization: Bearer <密钥>`。密钥以创建人的
  身份调用，操作会记入操作日志。不要带 Cookie。
- 仅开发环境（`app.env=dev`）可用，否则返回 403 / 80001。
- 请求和响应都是 JSON（本说明除外）。响应统一为
  `{"code":0,"message":"success","data":...,"requestId":"..."}`，`code` 非 0 即失败。
- 校验失败返回 422，`data.fields` 列出每个问题：`[{"field":"columns[1].type","message":"..."}]`，
  按提示改正后重试即可。
- 网络超时或没收到响应时，**原样重发同一个请求**是安全的：已经成功的建表和生成会返回上次的结果，
  不会重复执行。

## 标准流程

1. `GET /tables` 看已有的表，`GET /dicts` 看可绑定的数据字典。
2. `POST /tables` 建表（可先加 `"dryRun": true` 看生成的 SQL）。
3. `POST /preview` 预览要生成的文件、菜单和权限。
4. `POST /generate` 写入项目。
5. **重启后端**（新模块和新迁移在启动时加载；前端开发服务器会自动刷新）。
6. 超级管理员直接能看到新菜单；其他角色要在「系统管理 → 角色管理」里分配
   新权限（权限码见 preview 返回的 `permissions`）。
7. 生成的代码可以继续修改；改动生成文件后再次 generate 需要 `"force": true`
   才会覆盖，**覆盖会丢掉手工修改**，一般不要这么做。

## 1. 建表 `POST /tables`

只描述业务字段，下列标准字段会自动加上，不要写进 `columns`：

| 字段 | 说明 |
| --- | --- |
| `id` | bigint 自增主键 |
| `version` | 乐观锁版本号 |
| `created_at` / `updated_at` | 创建 / 更新时间 |
| `created_by` / `updated_by` | 创建人 / 更新人 |

可选的约定字段用开关加：

| 开关 | 加的字段 | 效果 |
| --- | --- | --- |
| `"status": true` | `status` smallint（1 启用 / 2 停用） | 列表显示状态开关、可按状态搜索 |
| `"sort": true` | `sort` int | 列表按 sort 排序 |
| `"dept": true` | `dept_id` 关联部门 | 启用部门数据权限 |
| `"softDelete": true` | `deleted_at` | 删除改为软删除，唯一索引只约束未删除的行 |

请求示例：

```json
{
  "name": "biz_products",
  "label": "商品",
  "status": true,
  "sort": true,
  "columns": [
    {"name": "code", "type": "string", "label": "商品编码", "length": 32, "required": true, "unique": true},
    {"name": "title", "type": "string", "label": "商品名称", "length": 100, "required": true},
    {"name": "category_id", "type": "ref", "ref": "biz_categories", "label": "分类"},
    {"name": "price", "type": "decimal", "label": "价格", "precision": 12, "scale": 2, "required": true, "default": "0"},
    {"name": "kind", "type": "int", "label": "类型", "required": true, "default": "1",
     "options": [{"value": "1", "label": "实物"}, {"value": "2", "label": "虚拟"}]},
    {"name": "on_sale", "type": "bool", "label": "上架"},
    {"name": "cover", "type": "image", "label": "封面"},
    {"name": "files", "type": "attachments", "label": "附件"},
    {"name": "launched_at", "type": "datetime", "label": "上市时间"},
    {"name": "remark", "type": "text", "label": "备注"}
  ]
}
```

返回 `data`：`migration`（迁移文件名）、`up` / `down`（SQL）、`applied`（已执行）、`notes`。
开发库还有没执行的迁移时返回 409 / 80004，不会建表；请用户先处理（执行迁移或删除那个文件）。
迁移文件写在 `server/migrations/app/NNNNNN_create_<表名>.up.sql / .down.sql`（业务迁移，独立编号），并已在开发库执行，
和手写的迁移一样随项目提交。

### 表规则

- `name`：小写字母开头，只含小写字母、数字、下划线，最长 30；不能以 `sys_` 开头
  （系统表专用），建议 `biz_` 或模块名做前缀，用复数，如 `biz_orders`。表必须是新表。
- `label`：中文表名，最长 20 字，不要含括号、冒号、逗号、等号。
- `columns`：1–60 个业务字段。

### 字段规则

| 属性 | 说明 |
| --- | --- |
| `name` | 小写蛇形，最长 28；不能用标准字段名（id、version、created_at、updated_at、created_by、updated_by、deleted_at、status、sort、dept_id） |
| `type` | 见下表 |
| `label` | 中文字段名（必填），最长 20 字，会成为页面上的标签；不要含括号、冒号、逗号、等号 |
| `required` | NOT NULL，表单必填 |
| `default` | 默认值：数字、`true`/`false`、`YYYY-MM-DD`、普通文字；datetime 只能是 `"now"` |
| `unique` | 唯一索引（string、int、bigint、date、ref） |
| `length` | string 长度，1–2000，默认 64 |
| `precision` / `scale` | decimal 精度，默认 12 / 2 |
| `ref` | ref 字段引用的表（须有 bigint id），字段名必须以 `_id` 结尾 |
| `options` | 固定选项（string、int、bigint），至少 2 个；值为 1–32 位字母数字 `_` `-`，int 字段须为整数。页面生成下拉框，数据库加 CHECK 约束 |

| type | 数据库类型（PostgreSQL / MySQL） | 页面控件 |
| --- | --- | --- |
| `string` | varchar(length) | 输入框 |
| `text` | text / mediumtext | 多行文本 |
| `int` / `bigint` | integer / bigint | 数字输入 |
| `decimal` | numeric(p, s) / decimal(p, s) | 数字输入（金额等，不要用浮点） |
| `bool` | boolean，默认 false | 开关 |
| `date` | date | 日期 |
| `datetime` | timestamptz / datetime(6) | 日期时间 |
| `json` | jsonb / json | JSON 文本 |
| `image` | varchar(512) | 图片上传 |
| `attachments` | jsonb / json 数组 | 多文件上传 |
| `ref` | bigint 外键 | 远程搜索下拉（显示被引用表的名称字段） |

迁移按开发库的数据库生成：PostgreSQL 写到 `server/migrations/app`，MySQL 写到
`server/migrations/app/mysql`。唯一字段（`unique`）的 string 长度最长 768。

选项值会出现在很多地方时（多张表共用），不要用 `options`，改为在「系统配置 → 数据字典」
建字典，字段用 string 或 int，生成时在 `fields` 里绑定 `dict`。

## 2. 预览与生成 `POST /preview`、`POST /generate`

```json
{
  "table": "biz_products",
  "module": "shop",
  "resource": "product",
  "name": "商品",
  "moduleTitle": "商城管理",
  "fields": [
    {"column": "title", "search": "like"},
    {"column": "kind", "search": "eq"},
    {"column": "price", "search": "range"},
    {"column": "remark", "list": false}
  ]
}
```

- `module`：Go 包名，小写字母开头的 2–31 位小写字母或数字。新模块会自动注册到
  `server/internal/app/modules.go`，并建一个目录菜单（标题 `moduleTitle`）；已存在的
  模块会加入新资源。不能用 auth、iam、devtools、core、app。建议每个业务领域一个新模块，不要往系统自带模块（system、audit、file、ops、notice）里加。
- `resource`：kebab-case，如 `product`、`sales-order`，决定接口路径和权限码。
- `name`：资源中文名，用于菜单标题和提示。
- `fields`：可选，只写需要改默认值的字段；默认值可用 `GET /fields?table=biz_products` 查看：
  - `label`：中文名（默认取字段注释）
  - `list`：是否在列表显示；`form`：是否出现在新增/编辑表单
  - `search`：`none` / `like`（包含）/ `eq`（等于）/ `range`（区间，数字和日期）
  - `dict`：数据字典编码；`"@comment"` 表示用建表时的 `options`；`""` 不用选项
  - `upload`：`image` / `files` / `""`
  - `refTable`、`refKey`、`relation`：没有外键时手动声明关联，如
    `{"column": "user_id", "refTable": "lg_user", "relation": "nickname"}`（`refKey` 默认 `id`）
- `keyMode`：可选，主键来源。`auto` 数据库自增；`generated` 由模块用雪花 ID 生成（bigint 主键不是自增时的默认值）；
  `input` 新增时填写、之后不能改（文本或整数主键的默认值，如 `cfg_key`、`level`）。
- `readOnly`：可选，`true` 时只生成列表、详情和导出（流水、订单、日志类表）。
- `children`：可选，随主表一起编辑的子表（多语言表）：
  `[{"table": "lg_item_i18n", "foreignKey": "item_id", "keyColumn": "lang", "keyDict": "lang", "label": "多语言"}]`。
  表单按字典 `keyDict` 的每一项列出一组输入，详情接口返回这些行。键也可以来自一张表（与 `keyDict` 二选一）：
  `"keyTable": "lg_language", "keyValueColumn": "code", "keyLabelColumn": "name", "keyFilter": "enabled = 1", "keySortColumn": "sort", "keyRequiredColumn": "required"`，
  `keyRequiredColumn` 为真的键必须填写。`keyFilter` 原样写进生成的代码，两种数据库都要用时写 `enabled = TRUE`（`enabled = 1` 只适用于 MySQL）。
  - `relation`：外键字段显示被引用表的哪一列；`""` 改为直接输入 ID
- `force`（仅 generate）：覆盖已存在的文件。

generate 返回 `files`（写入的文件）、`newModule`、`wired`、`notes`。**照 `notes` 做完剩下的步骤**
（通常只有重启后端）。生成的文件包括：

- `server/internal/modules/<module>/`：`module.go`（菜单、权限、路由注册）、`<resource>.go`（接口与查询）、
  `<resource>_integration_test.go`（集成测试）
- `web/src/api/<module>/<resource>.ts`、`web/src/views/<module>/<resource>/index.vue`（列表、搜索、新增/编辑、导出）

生成的接口（以 module=shop、resource=product、表 biz_products 为例，路径是资源名的复数，
不带模块名，均在 `/api/v1` 下，由后台页面用登录会话调用，不能用本接口的密钥调用）：

| 方法 | 路径 | 权限码 |
| --- | --- | --- |
| GET | `/products` | `shop:product:view`（分页、搜索、排序） |
| GET | `/products/export` | `shop:product:export`（导出 CSV） |
| GET | `/products/:id` | `shop:product:view` |
| POST | `/products` | `shop:product:create` |
| PUT | `/products/:id` | `shop:product:update`（带 version 乐观锁） |
| DELETE | `/products/:id` | `shop:product:delete` |

新增时没传的选填字段如果建表时设了 `default`，使用默认值；编辑（PUT）时没传则保持原值。
具体以 preview 返回的文件和权限为准。生成后要加业务逻辑时，直接修改生成的 Go / Vue 文件，
保持其中的结构（数据权限、乐观锁、操作日志的 Doc 说明）不变。

## 3. 其他接口

- `GET /tables`：已有数据表及其中文名、是否已生成。
- `GET /fields?table=<表名>`：表的字段与生成默认值（类型、中文名、列表/表单/搜索、可用的选项和上传方式、外键）。
- `GET /dicts`：数据字典（code、name、enabled）。
- `GET /guide`：本说明。

## 常见错误

| HTTP / code | 含义 | 处理 |
| --- | --- | --- |
| 401 / 20001 | 缺少密钥、密钥无效、过期或已撤销 | 请用户在「开发工具 → AI 接口」重新创建 |
| 403 / 20002 | 密钥的创建人被停用或没有「使用代码生成」权限 | 请管理员处理 |
| 403 / 80001 | 不是开发环境 | 只能在本地开发环境使用 |
| 409 / 80002 | 要生成的文件已存在 | 确认可以覆盖后加 `"force": true` |
| 409 / 80004 | 开发库还有没执行的迁移 | 请用户先执行迁移或删除未执行的迁移文件 |
| 422 / 10002 | 参数不符合规则 | 按 `data.fields` 逐条改正 |

## 示例（curl）

```bash
KEY=bak_xxx   # 用户给你的密钥
BASE=http://localhost:8848{{BASE}}   # 用户给你的完整基础地址
curl -s -H "Authorization: Bearer $KEY" $BASE/tables
curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d @table.json $BASE/tables
curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d @module.json $BASE/generate
```
