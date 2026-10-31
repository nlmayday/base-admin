# AI 接口（给 AI 编程助手用）

开发工具 → **AI 接口**：给 Claude Code、Cursor 等 AI 编程助手调用的 HTTP 接口。AI 用它按
[建表规范](table-guide.md)建业务表，再用代码生成器生成增删改查模块（后端接口、前端页面、菜单、
权限、集成测试），保证 AI 写的功能遵守框架约定，而不是各写各的。

## 使用步骤

1. 在 开发工具 → AI 接口 点「创建密钥」，填写用途，选有效期（1、7、30 天，默认 7 天；密钥都会过期）。
2. 弹窗里复制「给 AI 的说明」发给 AI。说明里有接口地址、密钥和读取使用说明的命令。
   **密钥只显示这一次**，库里只存 SHA-256。
3. AI 读取使用说明（`GET /api/v1/devtools/agent/guide`），按说明建表、预览、生成。
4. 重启后端，新菜单和接口生效；给需要的角色分配新权限。

任务做完就在列表里撤销密钥，撤销立即生效。

## 用 curl 直接调用

不经过 AI，自己用 curl 也能一步建好表和增删改查。下面的命令在本地开发环境跑通过：

```bash
KEY=bak_xxx                                        # 「创建密钥」弹窗里的密钥
BASE=http://localhost:8848/api/v1/devtools/agent   # 弹窗说明里的基础地址
J=(-s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json')

# 1. 建表：写出 server/migrations/app/NNNNNN_create_biz_items.{up,down}.sql 并在开发库执行
#    （先加 "dryRun": true 可以只看 SQL）
curl "${J[@]}" $BASE/tables -d '{
  "name": "biz_items", "label": "商品", "status": true,
  "columns": [
    {"name": "title", "type": "string", "label": "名称", "required": true, "unique": true},
    {"name": "kind", "type": "int", "label": "类型", "required": true, "default": "1",
     "options": [{"value": "1", "label": "实物"}, {"value": "2", "label": "虚拟"}]},
    {"name": "price", "type": "decimal", "label": "价格", "required": true, "default": "0"},
    {"name": "remark", "type": "text", "label": "备注"}
  ]
}'

# 2. 生成模块（先把 generate 换成 preview 可以只看要生成的文件、菜单和权限）
curl "${J[@]}" $BASE/generate -d '{
  "table": "biz_items", "module": "shop", "resource": "item",
  "name": "商品", "moduleTitle": "商城管理",
  "fields": [{"column": "title", "search": "like"}, {"column": "kind", "search": "eq"}]
}'

# 3. 重启后端（make run），菜单「商城管理 → 商品管理」和接口 /api/v1/items 就生效了
```

生成后会得到：

- 后端模块 `server/internal/modules/shop/`，已注册到 `server/internal/app/modules.go`，包括集成测试；
- 前端页面 `web/src/views/shop/item/` 和接口文件 `web/src/api/shop/item.ts`；
- 接口 `GET/POST /api/v1/items`、`GET/PUT/DELETE /api/v1/items/:id`、`GET /api/v1/items/export`，
  权限码 `shop:item:view/create/update/delete/export`。这些接口用登录会话调用，不能用 AI 接口的密钥调用。

字段类型、开关和 `fields` 的全部写法见 `GET $BASE/guide`（即 [agent_guide.md](../server/internal/modules/devtools/agent_guide.md)）。
请求失败时看返回的 `data.fields`，它会指出是哪个字段、哪里不对。

## 接口

基础路径 `/api/v1/devtools/agent`，每个请求带 `Authorization: Bearer <密钥>`：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/guide` | 给 AI 的使用说明（Markdown），内容见 [agent_guide.md](../server/internal/modules/devtools/agent_guide.md) |
| GET | `/tables` | 数据表列表 |
| GET | `/fields?table=` | 表字段与生成默认值 |
| GET | `/dicts` | 数据字典 |
| POST | `/tables` | 按结构化描述建表：生成 `server/migrations/app/NNNNNN_create_<表>.{up,down}.sql`（业务迁移轨道）并在开发库执行；`dryRun` 只返回 SQL |
| POST | `/preview` | 预览生成结果 |
| POST | `/generate` | 生成模块到项目 |

## 安全设计

- **只在开发环境可用**（`app.env=dev`），其他环境返回 403 / 80001；密钥管理页在任何环境都能打开。
- 密钥以**创建人的身份**调用：每次请求重新读取创建人的状态和权限，创建人被停用或失去「使用代码生成」
  权限后密钥立即失效；操作记入操作日志（操作人为创建人）。
- 只接受结构化的表描述，**不接受 SQL**。表名、字段名、类型、默认值、选项都经过校验，中文名作为注释写入时会转义。
  建表 SQL 先在回滚的事务里试执行，失败不会留下文件或脏迁移。
- 建表前要求开发库的业务迁移版本等于 `server/migrations/app` 里最后一个迁移，否则返回 409 / 80004，
  所以建表只会执行它自己生成的那个迁移，不会顺带执行手写的、还没执行的迁移。
- 不是开发环境时，在查密钥之前就返回 403，生产环境不会因为这些请求去查库。
- 只要请求带 `Authorization: Bearer` 且没有会话 Cookie，就不做 CSRF 校验：浏览器无法给跨站请求加这个请求头，
  没有 Cookie 也就没有可被冒用的会话。带会话 Cookie 的请求仍按原规则校验。
- 管理密钥需要权限 `devtools:agent:view`（「管理 AI 接口密钥」），可以查看和撤销所有人的密钥。

## 注意

- 生成的迁移文件和代码都写进仓库，和手写的一样需要评审、提交。
- 已生成的文件默认不覆盖；`force: true` 会覆盖手工修改。
- 可以安全重试：建表时同一个表、同样的定义再提交一次，直接返回上次的结果；生成时同一用户 5 分钟内的相同请求
  也返回上次的结果。生成同一时间只处理一个请求，写文件中途失败会把已写的文件恢复原样。
- 生成的新增接口里，不传「有默认值的选填字段」时使用数据库默认值；编辑时不传则保持原值。
- 迁移在开发库执行后，其他环境照常通过 `make migrate-up`（或部署时的迁移）执行。
