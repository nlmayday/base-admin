# base-admin

简体中文 | [English](README.en.md)

可复用的通用后台管理系统模板：后端 Go + Gin，前端 Vue 3 + Element Plus（基于 pure-admin-thin）。架构基线见 [docs/rebuild-plan-v2.md](docs/rebuild-plan-v2.md)，P0 接口约定见 [docs/api-p0.md](docs/api-p0.md)，P1–P3 功能规划见 [docs/p1-p3-plan.md](docs/p1-p3-plan.md)。

## 功能

| 菜单 | 功能 |
|---|---|
| 工作台 | 问候语、按权限显示的统计卡片、快捷入口、系统状态、近 7 天登录与操作趋势 |
| 系统管理 | 用户（支持 CSV 导入：下载模板、逐行提示错误、全部成功才写入）、角色（菜单、按钮、接口权限与数据范围）、部门、岗位、菜单（图标从内置的离线图标库中选择） |
| 系统配置 | 数据字典（可设标签样式）、系统参数（带类型和修改历史）、基础设置（站点名称与 Logo、上传规则、日志保留天数） |
| 消息中心 | 带附件的通知公告：草稿、发布、撤回；业务模块发给指定用户的个人消息（点击跳转到业务页面）；两者都在顶栏铃铛（显示未读数）和「我的消息」中查看 |
| 安全中心 | 在线会话（按会话或按用户强制下线）；安全策略：密码复杂度、验证码（关闭 / 失败 N 次后 / 始终）、账号锁定、IP 白名单、允许登录时段（可按角色豁免）；业务模块可以注册自己的登录检查 |
| 日志审计 | 登录日志与操作日志（接口说明、耗时、结果、请求 ID、敏感字段脱敏），支持统计和 CSV 导出 |
| 资源管理 | 供业务表单使用的图片和附件上传组件（登录用户即可上传，链接需要登录但不需要文件权限）；媒体库（分类、网格和表格视图、预览、签名链接、拖拽上传并检测文件内容）与存储概览；支持本地磁盘或 S3 / MinIO |
| 运维管理 | 定时任务（cron 表达式预览、重试、超时、多实例下每次只执行一次、手动执行）与执行记录；服务器与应用监控 |
| 开发工具 | 由路由注册表自动生成的接口文档（可导出 OpenAPI 3 JSON）；代码生成器（仅开发环境）：选一张表，设置每个字段的中文名、是否在列表和表单中显示、搜索方式，即可生成可用的模块并自动接入项目。**AI 接口**：给 AI 编程助手的密钥，AI 按同样的约定通过 HTTP 建表并生成模块（[docs/ai-agent-api.md](docs/ai-agent-api.md)） |

设计要点：
- **模块化单体**：每个功能都是一个 `module.Module`，自带权限码、菜单、路由和定时任务。启动时校验，未加保护或未声明的路由会导致启动失败。
- **Redis 不透明会话**：通过 PostgreSQL 中的会话代数实现撤销；CSRF 防护采用 Origin 校验加绑定会话的双重提交令牌。
- **权限**：权限码之间有依赖关系，每个查询都带数据范围，角色和部门有防越权规则。

## 环境要求

- Go：版本见 `server/go.mod`，`GOTOOLCHAIN=auto` 会自动下载。
- Docker 和 Compose v2。
- 前端需要 Node ≥ 20.19、pnpm ≥ 9。

## 快速开始

```bash
make deps-up          # 启动 PostgreSQL 127.0.0.1:15432、Redis 127.0.0.1:16380
make migrate-up       # 执行数据库迁移（建表和初始数据）
make run              # 后端接口 :8080，监控指标 127.0.0.1:9090
make web-install
make web-dev          # 前端 http://localhost:8848（/api 代理到 API_URL）
```

用 `admin` 登录，密码是 `ADMIN_BOOTSTRAP_ADMIN_PASSWORD` 的值（Makefile 中的开发默认值：`Admin@dev-2026`），首次登录必须修改密码。

如果 8080 端口被占用，把后端换个端口，并让前端指向它：

```bash
ADMIN_HTTP_ADDR=:18080 make run
API_URL=http://127.0.0.1:18080 make web-dev
```

也可以用 MySQL 8.4 代替 PostgreSQL：`make deps-up-mysql`、`make migrate-up-mysql`、`make run-mysql`，写法和限制见 [docs/mysql.md](docs/mysql.md)。代码生成器和 AI 接口建表在两种数据库上都能用。

`make help` 列出所有命令。`make ci` 依次执行：构建、lint、后端单元测试和集成测试、前端 lint 和类型检查、前端构建。

### 初始数据

不用手动导入数据，但**迁移要先执行**：`make run` 发现数据库不是最新版本时会直接退出并提示先执行迁移。

- `make migrate-up` 建表，并写入默认数据：数据字典、安全策略、内置定时任务（日志清理、文件清理、个人消息清理等）。
- 后端每次启动时同步所有模块的权限码、菜单和系统参数，所以新增的菜单和权限重启后就会出现。
- 数据库里还没有用户时，会自动创建「超级管理员」角色和 `admin` 账号，密码取自 `ADMIN_BOOTSTRAP_ADMIN_PASSWORD`。没有设置这个变量时启动会报错。

不会生成演示业务数据。

## 配置

优先级从低到高：内置默认值、可选的 YAML 文件（`-config` 或 `ADMIN_CONFIG`）、`ADMIN_*` 环境变量。[server/configs/config.example.yaml](server/configs/config.example.yaml) 列出了所有配置项；未知配置项、不带单位的时长都会被拒绝。

密钥类配置只从环境变量读取，配置文件里出现这些项时服务拒绝启动：

| 变量 | 用途 |
|---|---|
| `ADMIN_DATABASE_URL` | 数据库连接地址（必填）：`postgres://…` 或 `mysql://…`，协议头决定用哪种数据库 |
| `ADMIN_REDIS_PASSWORD` | Redis 密码 |
| `ADMIN_BOOTSTRAP_ADMIN_PASSWORD` | 没有任何用户时用它创建第一个超级管理员（12–72 个字符） |
| `ADMIN_FILE_SIGNING_KEY` | 文件签名链接的 HMAC 密钥，至少 32 字节（生产环境必填） |
| `ADMIN_FILE_S3_ACCESS_KEY` / `ADMIN_FILE_S3_SECRET_KEY` | `file.driver=s3` 时的 S3 / MinIO 凭证；两个都不填则使用 AWS / MinIO 环境变量或实例角色 |

运行时设置保存在数据库中，在界面上修改：「基础设置」管站点名称、Logo、上传规则和日志保留天数；「安全中心 → 安全策略」管密码、验证码和锁定规则。

## 部署

`deploy/docker/compose.prod.yaml` 启动整套服务：
- PostgreSQL 和 Redis（设置了密码，`noeviction`，开启 AOF）；
- 一次性的 `migrate` 服务；
- 后端：distroless 非 root 镜像，根文件系统只读，不授予任何 capability；健康检查执行 `admin health ready`（检查 PostgreSQL 和 Redis 能否连通）；
- nginx：提供前端页面（`/static/` 下带哈希的文件缓存一年，`index.html` 不缓存）并代理 `/api`；健康检查 `/healthz`。

镜像版本固定到补丁版本。依赖升级直接提交到 main，不走 PR；`security.yml` 每周扫描漏洞，发现问题时再升级。

```bash
cp deploy/docker/prod.env.example deploy/docker/prod.env   # 填好每一项
docker compose -f deploy/docker/compose.prod.yaml --env-file deploy/docker/prod.env up -d --build
```

用 MySQL 部署时换成 `deploy/docker/compose.prod.mysql.yaml` 和 `prod.mysql.env.example`，其余相同：

```bash
cp deploy/docker/prod.mysql.env.example deploy/docker/prod.mysql.env   # 填好每一项
docker compose -f deploy/docker/compose.prod.mysql.yaml --env-file deploy/docker/prod.mysql.env up -d --build
```

本地存储的上传文件保存在 `files` 卷中。部署多个后端副本时请用 `file.driver=s3`；定时任务在多实例下本来就只执行一次。

上线检查清单：
- 设置 `ADMIN_APP_ENV=prod`。
- Cookie 名用 `__Host-sid`，并开启 `auth.cookie_secure=true`（生产环境强制）。
- `auth.allowed_origins` 设为真实的前端地址。
- `http.trusted_proxies` 设为负载均衡或 Ingress 所在网段。
- 设置 `http.drain_delay`。
- 每套部署使用独立的 Redis 逻辑库，淘汰策略用 `noeviction` 或 `volatile-*`。
- 设置 `ADMIN_FILE_SIGNING_KEY`。
- 监控端口（`metrics.addr`）不要暴露到公网。
- 定期备份 PostgreSQL（以及 `files` 卷或 S3 存储桶），并演练恢复；执行 `migrate up` 前先做快照。这套部署不包含备份任务。

## 命令

```text
admin serve                      启动后端（默认）
admin migrate up                 执行未执行的迁移（先脚手架轨道，再业务轨道）
admin migrate version            显示两条轨道的版本（dirty 时退出码为 1）
admin migrate <track> down <n>   回滚某条轨道的 n 个迁移
admin migrate <track> force <v>  从 dirty 状态恢复（v 为已有版本号，-1 表示无）
gen -table … -module …           从数据表生成模块（go run ./cmd/gen -h）
```

迁移分两条轨道，各自编号、各有版本表：`server/migrations` 是脚手架自己的（`schema_migrations`），`server/migrations/app` 是业务的（`app_schema_migrations`）。合并上游时两边新增的迁移不会撞号。轨道名是 `base` 或 `app`，`up` 和 `version` 也可以只指定一条，如 `admin migrate app up`。

数据库不是最新版本时 `serve` 拒绝启动，所以部署时要单独执行一步 `migrate up`。启动时先校验所有模块，再同步权限码、菜单和初始数据。第一次收到 SIGTERM 或 Ctrl-C 时优雅停止：处理完进行中的请求，等正在执行的任务结束，写完操作日志；第二次收到信号立即退出。

## 用这个模板开始新项目

```bash
scripts/new-project.sh github.com/acme/crm/server "Acme CRM" acme-crm
```

脚本会替换所有地方的 Go 模块路径（import、代码生成器模板、Dockerfile、lint 配置），修改产品名和 Compose 项目名，最后检查后端能否编译。

## 添加业务模块

详见 [docs/module-guide.md](docs/module-guide.md)。简单说：
1. 执行 `make new-migration NAME=create_<表名>`，在 `server/migrations/app` 下得到带标准字段的建表模板；填好业务字段和注释（[docs/table-guide.md](docs/table-guide.md)），再执行 `make migrate-up`。
2. 在「开发工具 → 代码生成」中：
   - 选择数据表；
   - 设置每个字段的中文名、是否在列表和表单中显示、下拉选项的来源（字段注释或数据字典）、搜索方式（模糊、精确或范围）；
   - 点击 **生成到项目**。

   会写入后端和前端文件并自动接入项目。命令行 `go run ./cmd/gen … -fields '<JSON>'` 效果相同。
3. 重启后端，把新的权限码分配给角色。

第 1–2 步也可以通过 HTTP 接口完成：在「开发工具 → AI 接口」创建密钥，然后
- 把复制的说明发给 AI 编程助手，AI 会读取使用说明，建表（包括迁移文件）并生成模块；
- 或者自己用 curl 调用，两条命令就能建好表和增删改查：

  ```bash
  curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    $BASE/tables -d '{"name":"biz_items","label":"商品","columns":[{"name":"title","type":"string","label":"名称","required":true}]}'
  curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    $BASE/generate -d '{"table":"biz_items","module":"shop","resource":"item","name":"商品","moduleTitle":"商城管理"}'
  ```

  `BASE` 是 `http://localhost:8848/api/v1/devtools/agent`，只在开发环境可用。完整示例和字段写法见 [docs/ai-agent-api.md](docs/ai-agent-api.md#用-curl-直接调用)。

## 测试

- `make test`：单元测试，不依赖任何外部服务。
- `make test-integration`：再加上带 `integration` 标签的测试，连接 Compose 启动的服务运行。
  - 每个测试创建并删除自己的 PostgreSQL 数据库，并临时租用一个 Redis 逻辑库，结束时清空归还。
  - 只会用 1–15 号库里空的或测试留下的库，不碰 0 号库和存有其他数据的库，所以可以和开发环境共用 Redis，也可以同时跑多份测试。开发服务请用 0 号库（默认）或已有数据的库。
  - 缺少环境变量时这些测试会失败，而不是跳过。
- `make web-check`：前端 lint（只检查）、类型检查和 Vitest 单元测试（`pnpm test`；开发时用 `pnpm test:watch`）。
- `make codegen-web-check`：为几种表结构生成前端代码，再格式化、lint 和类型检查。
- `make e2e`：Playwright 端到端测试，需要先启动后端。
  - `E2E_ADMIN_USERNAME`、`E2E_ADMIN_PASSWORD` 设为一个已经改过初始密码的管理员；也可以在全新数据库上同时设置 `E2E_BOOTSTRAP_PASSWORD`，测试会先把初始密码改成 `E2E_ADMIN_PASSWORD`（CI 就是这样做的）。
  - 测试只登录这个管理员一次并复用会话；登录、登出相关的测试使用临时用户。
- CI（`.github/workflows/ci.yml`）：后端构建、lint 和集成测试；前端检查（`pnpm lint` 只检查，本地用 `pnpm lint:fix` 修复）和构建，再执行 `git diff --exit-code`；在全新数据库上跑 Playwright 端到端测试。`security.yml` 每周以及 main 分支有提交时运行 govulncheck 和 `pnpm audit --prod`。
- 提交代码时，git 钩子会自动格式化并检查前端改动的文件，提交说明须符合 `feat: ...`、`fix: ...` 这样的格式。
