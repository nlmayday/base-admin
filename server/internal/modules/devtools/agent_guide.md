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
- `menuTitle`、`menuSort`：可选，页面菜单标题（默认「中文名 + 管理」）和排序（0–9999，小的在前，默认 1）。
  菜单的标题和排序只在第一次写入数据库时生效，之后在菜单管理里改。
- `name`：资源中文名，用于菜单标题和提示。
- `fields`：可选，只写需要改默认值的字段；默认值可用 `GET /fields?table=biz_products` 查看：
  - `label`：中文名（默认取字段注释）
  - `list`：是否在列表显示；`form`：是否出现在新增/编辑表单；`export`：是否导出（默认导出主键和列表显示的列）
  - `search`：`none` / `like`（包含）/ `eq`（等于）/ `range`（区间，数字和日期）
  - `dict`：数据字典编码；`"@comment"` 表示用字段注释中的选项（`1=通知,2=公告` 或 `1正常 2冻结`）；`""` 不用选项。
    只读模块的字段也可以用，`status` 用了选项后不再是启用/停用；是/否字段只在不进表单时可用，字典值为 `1`、`0`
  - `upload`：`image` / `files` / `""`
  - `refTable`、`refKey`、`relation`：没有外键时手动声明关联，如
    `{"column": "user_id", "refTable": "lg_user", "relation": "nickname"}`（`refKey` 默认 `id`）。本列与关联列同为 bigint 或同为文本
    （如 `{"column": "game_type", "refTable": "lg_game_family", "refKey": "family", "relation": "name"}`，显示列也可以就是 `family` 本身）；
    显示列可以是整数列（如 `"relation": "short_id"`，按编号精确匹配，走索引）；列表搜索栏能选到已删除的记录，表单不能
  - `createOnly`：`true` 时新增时填写、编辑时只读（如游戏编码），编辑接口不修改它
  - `hint`：表单输入框下方的说明文字（最长 200 字）
  - 主键也可以 `"list": true` 进列表（如流水号、订单号），并可 `"search": "eq"`
  - `format`：列表显示格式，`coins` 千分位、`signed` 千分位带符号（正绿负红）、`cents` 美分显示成 $x.xx、`permille` 千分比显示成 +x%、
    `milli` ÷1000 保留 2 位（`suffix` 加后缀，如 `"x"`）、`image` 缩略图；导出仍为原值
  - `relationExtra`：关联表的第二个显示列，如 `"relation": "short_id", "relationExtra": "nickname"` 显示为 100001（玩家甲）
  - `searchDefault`：搜索项默认值（重置回到它）；`default`：新增表单默认值；`min`、`max`、`step`：数字输入范围；`placeholder`
  - `showWhen`：`{"column": "fill_type", "in": [1]}`，另一字段取这些值时才显示
  - `required`：`false` 时 NOT NULL 文本可保存空串；文本字段保存前去首尾空格
  - `showIf`：`"feature:vip"`，开关关闭时列表和表单都不显示、编辑保留原值；开关由模块手写的
    `func (m *Module) featureEnabled(ctx context.Context, name string) bool` 提供（不写则都开）
- `keyMode`：可选，主键来源。`auto` 数据库自增；`generated` 由模块用雪花 ID 生成（bigint 主键不是自增时的默认值）；
  `input` 新增时填写、之后不能改（文本或整数主键的默认值，如 `cfg_key`、`level`）。
- `readOnly`：可选，`true` 时只生成列表、详情和导出（流水、订单、日志类表）。
- `softDelete`：可选，`false` 时 `deleted_at` 是普通时间字段（如注销时间）：列出全部行、可搜索，删除即删除行。
- `noCreate`、`noDelete`：可选，`true` 时不生成新增 / 删除的接口、权限码和按钮（如游戏只能下架不能删）。
- `orderBy`：可选，列表和导出的排序，如 `"created_at DESC, id DESC"`，也可比较值 `"status = 0 DESC, id"`（大表请选有索引的列）；默认 `sort` 后主键倒序。
- `deleteConfirm`：可选，删除确认文案，`{列名}` 换成该行的值。
- `children`：可选，随主表一起编辑的子表（多语言表）：
  `[{"table": "lg_item_i18n", "foreignKey": "item_id", "keyColumn": "lang", "keyDict": "lang", "label": "多语言"}]`。
  表单按字典 `keyDict` 的每一项列出一组输入，详情接口返回这些行。键也可以来自一张表（与 `keyDict` 二选一）：
  `"keyTable": "lg_language", "keyValueColumn": "code", "keyLabelColumn": "name", "keyFilter": "enabled = 1", "keySortColumn": "sort", "keyRequiredColumn": "required"`，
  `keyRequiredColumn` 为真的键必须填写。`keyFilter` 原样写进生成的代码，两种数据库都要用时写 `enabled = TRUE`（`enabled = 1` 只适用于 MySQL）。
  保存只替换请求中出现的键（全部为空的行删除），其他键的行保留（如未启用语言的翻译）。
  子表的 `fields` 可配置 `label`、`form`（`false` 不进表单）、`upload`（`image`）、`hint`；`"listKeys": true` 时列表增加「已填<label>」列，列名可用 `listKeysLabel` 另起（最长 20 字）。
  `columns`：`[{"key": "en", "field": "name", "label": "名称(英文)"}]` 把某个键的字段作为列表列；子表文本字段 `"search": "like"` 增加列表搜索项（EXISTS 子查询）；
  `requiredFields`：非空行和必填键的行中这些字段必须填写（去首尾空格后），表单中未填的必填键标红；`hint`：子表标题下的说明；子表字段可配 `placeholder`。
  - `relation`：外键字段显示被引用表的哪一列；`""` 改为直接输入 ID
- `force`（仅 generate）：覆盖已存在的文件。

generate 返回 `files`（写入的文件）、`newModule`、`wired`、`notes`。**照 `notes` 做完剩下的步骤**
（通常只有重启后端）。生成的文件包括：

- `server/internal/modules/<module>/`：`module.go`（菜单、权限、路由注册）、`<resource>.go`（接口与查询）、
  `<resource>_integration_test.go`（集成测试）
- `web/src/api/<module>/<resource>.ts`、`web/src/views/<module>/<resource>/index.vue`（列表、搜索、新增/编辑、导出）
- 扩展文件 `<resource>_ext.go`、`components/<Resource>Extension.vue`：只在不存在时写入，`force` 也不覆盖，
  自定义逻辑写在这里：`<resource>Restrict`（所有查询的固定条件；列表计数不带关联表，只用 `t.<列>`）、
  `<resource>BeforeUpdate(ctx, tx, old, req)` / `<resource>BeforeDelete(ctx, tx, old)`（写入或删除前在同一事务中执行，
  `old` 为修改前的行，返回错误即回滚，如「活动开始后目标不能改」）、`<resource>AfterSave` / `<resource>AfterDelete`
  （写入或删除后在同一事务中执行，返回错误即回滚）、`<resource>AfterCommit`（事务提交后执行，如通知其他服务刷新缓存；
  出错只记日志，不影响已成功的保存）；生成的新增、编辑、删除会把操作日志的「操作对象」记为 `<resource>:<主键>`，
  `_ext.go` 里自定义的写接口也调用 `set<Resource>Target(c, key)`；Extension.vue 渲染在列表工具栏和每行「操作」列，还可导出
  `cellColumns`（接管某些列的渲染，place="cell"）、`hasHeader`（表格上方说明区）、`hasExpand`（展开行）、
  `canEdit(row)` / `canDelete(row)`（返回 false 不显示编辑 / 删除）
- 列表页读地址栏参数：与搜索字段同名的参数（如 `?userId=1001`）自动填入并查询；时间列的日期范围按 UTC 日
- 模块需要的角色可在 `module.go` 的 `Roles()` 中声明（`module.RoleDef`，权限写权限码或 `game:*` 这样的前缀）：
  启动时不存在则创建并授权，之后由管理员维护；新版本新增的权限码会补授

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
导出超过「基础设置 → 导出行数上限」（默认 10 万）时返回 422「数据量过大…请缩小查询范围」，不截断；时间列为 UTC，表头标注 (UTC)。
具体以 preview 返回的文件和权限为准。生成后要加业务逻辑时，优先写在扩展文件中（重新生成不会丢）；
直接改生成的 Go / Vue 文件时保持其中的结构（数据权限、乐观锁、操作日志的 Doc 说明）不变。

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
