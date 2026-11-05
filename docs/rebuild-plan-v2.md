# Gin + Vue 通用后台管理系统 — v2.0 精简架构方案

> 版本：v2.4
> 日期：2026-10-30
> 前序：v1.0（`gin_admin_rebuild_plan.md`）。v1.0 的功能需求仍然有效，本版调整架构决策、阶段划分和部分功能形态。
> v2.1 修订：页面访问权限码统一为 `view`；新增权限依赖（`Requires`）；权限缓存版本号改为事务内递增并单调同步到 Redis；前端统一权限入口。
> v2.2 修订：版本同步改为准确的"有界最终一致"表述；会话撤销改为会话代数机制并支持失败关闭；确定 Auth Snapshot 结构与多角色数据范围的合并语义；明确数据范围的配置粒度。
> v2.4 修订：记录 P0 实施中的偏差与补充（第 17 节），包括数据范围策略提前到 P0 的原因。
> v2.3 修订：会话代数的权威值移到 PG（Redis 只做单调镜像）；写明跨存储失败语义、状态转换与测试场景；补充数据范围边界；新增第 16 节 P0 冻结约束。
> 定位：自研 Go + Gin 后端，前端基于 pure-admin-thin，以**项目模板**形式供下游业务项目复用。

---

## 0. 相对 v1.0 的变更摘要

| 类别 | v1.0 | v2.0 | 原因 |
|---|---|---|---|
| 权限模型 | 菜单/按钮权限码 + Casbin API 授权 | **统一权限码 + RBAC**，移除 Casbin | 消除两套授权体系和双写 |
| API 管理 | 手工维护 API 资源表 | **路由注册自动产生，只读展示**，不落表 | 不会再出现"路由存在但未登记" |
| 认证 | JWT + 可撤销 Refresh Token | **Opaque Token + Redis Session** | 踢人、停用即失效都需要有状态会话 |
| Token 存储 | 待定 | **HttpOnly + Secure Cookie** + CSRF 防护 | 前端 JS 无法读取会话凭据 |
| 模块注册 | P3 插件能力 | **P0 核心架构** | 核心模块本身也通过注册接入 |
| 数据权限 | P1 | **P0 定义 Scope 接口**，P1 实现各策略 | 避免 P1 时重写全部查询 |
| 组织权限 | 多个独立模块 | **IAM 单模块**（内部按子包划分） | user/role/dept/menu 强耦合 |
| 数据库 | MySQL / PostgreSQL 待定 | **PostgreSQL** | 部分唯一索引、物化路径查询、事务性 DDL |
| 多租户 | 预留扩展 | **v1 明确不支持** | 不做半成品的预留 |
| 日志 | 登录/操作/数据访问/异常四类表 | **登录日志 + 操作审计**，异常走 Zap | 去除重复存储 |
| 监控 | gopsutil 页面 + Prometheus | **Health Check + Prometheus Metrics** | 展示交给 Grafana |
| 代码生成 | Web 生成器 + 历史/差异/回滚 | **开发期 CLI**，变更由 Git 管理 | 生产服务不写源码 |
| 插件 | 插件列表、安装卸载、版本 | **编译期业务模块**，不做插件市场 | 编译期注册没有运行时安装的概念 |

**v1.0「明确不做」清单保持不变**：AI/MCP/API CLI 化、工作流审批、通知中心、报表中心。

---

## 1. 目标与定位

### 1.1 重写目的

- 自主掌控架构：权限、会话、模块边界由我们定义，而不是继承上游框架的历史包袱。
- 降低耦合：模块通过注册接口接入核心，核心不感知具体业务。
- 方便长期复用：作为新后台项目的起点，复制即用。

### 1.2 下游使用方式

- **项目模板**：下游项目 fork/复制本仓库后自行演进。
- **暂不提供**稳定的 Go Module 公共 API，也不承诺下游可无冲突合并上游更新。
- 为降低下游合并成本：核心代码（`internal/core`、`internal/platform`）与业务模块（`internal/modules/*`）目录严格分离，下游业务只新增模块目录，尽量不改核心。

### 1.3 v1 明确不支持

- 多租户（任何表都不带 `tenant_id`）
- 动态插件 / 运行时加载代码
- 角色继承（v1.0 中为"可选"，v2.0 移出；需要时用多角色组合代替）
- 第三方登录 / SSO（后续可作为独立模块）

---

## 2. 技术选型

| 层级 | 技术 | 备注 |
|---|---|---|
| 语言/框架 | Go（锁定小版本）+ Gin | |
| ORM | GORM + pgx 驱动 | 复杂查询可用原生 SQL；**禁用 AutoMigrate** |
| 数据库 | PostgreSQL 16+ | |
| 迁移 | golang-migrate（SQL 文件） | 每个迁移提供 down |
| 缓存/会话 | Redis（go-redis） | 会话、权限缓存、限流、分布式锁 |
| 密码哈希 | Argon2id（`golang.org/x/crypto/argon2`） | 参数按 OWASP 建议起步：m=19MiB, t=2, p=1 |
| 日志 | Zap | JSON 输出，关联 request_id |
| 配置 | Viper | 敏感项只从环境变量/密钥管理读取 |
| 校验 | go-playground/validator | 统一转换为 422 错误格式 |
| 指标 | prometheus/client_golang | `/metrics` 仅内网或鉴权访问 |
| API 文档 | swaggo（P2） | |
| 测试 | testing + testify | 集成测试使用 `integration` 构建标签，连接 Docker Compose 提供的真实 PG/Redis，每个测试创建独立的临时数据库 |
| 前端 | Vue 3 + TS + Vite + Element Plus + Pinia | 基于 pure-admin-thin，锁定 tag/commit |

**已移除**：Casbin、golang-jwt、gopsutil、robfig/cron（推迟到 P2 再评估）。

---

## 3. 系统架构

```text
Browser (pure-admin-thin)
        │  同源：/ 静态资源，/api 反向代理
        ▼
     Nginx (HTTPS)
        │
        ▼
  Gin HTTP API ──────────────────────────────────────────────┐
   中间件链：RequestID → AccessLog → Recover → SecurityHeaders │
             → BodyLimit → Session(加载 Actor) → CSRF          │
             → Access(Public / Authenticated / Perm)         │
        │                                                    │
  Handler → Service → Repository ──(DataScope)──► PostgreSQL │
        │                                                    │
        └────────────► Redis（Session / 权限缓存 / 限流）◄─────┘

  启动期：Module Registry → 启动校验 → 权限/菜单同步 → 种子数据
  旁路：  /health/live /health/ready，/metrics（独立端口），Zap 结构化日志
```

**架构约束**

- 模块化单体，单一可执行文件。
- Service 不依赖 `*gin.Context`；当前用户（Actor）通过 `context.Context` 传递。
- 模块之间只能通过对方暴露的 Service 接口调用，不能直接访问对方的 Repository 或表。
- 前后端**同源部署**（开发期使用 Vite proxy），这是 Cookie 会话和 SameSite 策略的前提。

---

## 4. 模块注册机制（P0）

### 4.1 模块接口

```go
// internal/core/module/module.go
type Module interface {
    Name() string                 // 模块名，同时是权限码前缀，如 "iam"
    Permissions() []perm.Def      // 本模块声明的全部权限码
    Menus() []menu.Def            // 本模块提供的菜单（以稳定 code 标识）
    Routes(r *httpx.Router)       // 注册路由，必须声明访问级别
    Seed(ctx context.Context, tx *gorm.DB) error // 幂等的初始化数据
}
```

```go
// main.go / app wiring：显式列出启用的模块，编译期确定
modules := []module.Module{
    auth.New(deps),
    iam.New(deps),
    // 下游业务模块在此追加
}
```

模块启停通过配置项 `modules.<name>.enabled` 控制；被禁用模块的路由不注册，其菜单和权限标记为不可用，但已有授权数据不删除。

### 4.2 路由访问级别

`httpx.Router` 包装 Gin，**访问级别是必填参数**，不声明就无法注册：

```go
func (h *UserHandler) Routes(r *httpx.Router) {
    g := r.Group("/users")
    g.GET("",        access.Perm("iam:user:view"),   h.List)
    g.GET("/:id",    access.Perm("iam:user:view"),   h.Get)
    g.POST("",       access.Perm("iam:user:create"), h.Create)
    g.PUT("/:id",    access.Perm("iam:user:update"), h.Update)
    g.GET("/options", access.Authenticated(),        h.Options) // 下拉选项，只返回最小字段
}
```

| 级别 | 含义 |
|---|---|
| `access.Public()` | 无需登录。启动时打印完整的公开路由清单，便于审查 |
| `access.Authenticated()` | 已登录即可，用于 `/me`、下拉选项等 |
| `access.Perm(code)` | 需要该权限码 |
| `access.AnyPerm(codes...)` | 拥有任一即可，用于跨页面复用的接口 |

Router 在注册时把 `(method, path, access, module)` 记入注册表，供启动校验、「API 管理」只读页和越权测试使用。

### 4.3 启动校验

以下任一项不通过则**拒绝启动**：

1. 权限码格式非法：须匹配 `^[a-z][a-z0-9-]*(:[a-z][a-z0-9-]*){2}$`，且前缀等于所属模块名。
2. 权限码在多个模块中重复声明。
3. 路由引用了未声明的权限码。
4. 菜单引用了未声明的权限码；菜单 code 重复；父菜单 code 不存在。
5. Gin 引擎中存在 `/api` 前缀下、但未经 `httpx.Router` 注册的路由（防止绕过包装器直接用 Gin 注册）。
6. 权限依赖（`Requires`）引用了未声明的权限码，或依赖关系存在环。
7. 页面菜单（`type=menu`）没有引用权限码，或引用的不是 `view` 动作。

以下情况**只告警**：

8. 声明了但没有任何路由使用的权限码（可能是纯前端按钮，需人工确认）。
9. 数据库中存在、代码中已移除的权限码：标记为 `deprecated`，不自动删除角色授权。

### 4.4 种子与同步规则

- **Schema 只由 migrations 管理**；种子数据只做幂等的 upsert。
- 权限码：启动时按代码声明同步到 `sys_permissions`（代码是唯一事实来源）。
- 菜单：按 `code` 插入不存在的项。对模块来源的菜单，`path`、`component`、`permission_code`、`parent` 由代码决定，每次启动覆盖；`title`、`icon`、`sort`、`hidden` 允许在后台修改，启动时不覆盖。
- 内置超级管理员角色和初始管理员账号：仅在不存在时创建；初始密码从环境变量读取，首次登录强制修改。

---

## 5. 权限模型

### 5.1 概念

```text
用户 ──N:N── 角色 ──N:N── 权限码
                │
                └── 数据范围（DataScope）

菜单 ──(可选) 引用一个权限码：用户拥有该权限码时菜单可见
```

- **授权的唯一事实来源**是 `sys_role_permissions(role_id, permission_code)`。
- 菜单不参与授权，只是 UI 结构。菜单可见规则：
  - 页面菜单：用户拥有它的 `permission_code`。
  - 目录：至少一个子菜单可见。
  - 外链：如果设置了权限码则按权限码判断，否则对所有已登录用户可见。
- 按钮不再是菜单节点，而是权限码。`perm.Def` 中的 `Menu` 字段把权限码挂到某个菜单下，用于授权页面按"菜单 → 权限"的树形展示。
- 同一页面的每个操作独立授权。不同角色访问同一页面时，看到的按钮各不相同，不需要为每个角色单独做页面。

```go
perm.Def{Code: "iam:user:view",   Name: "查看用户", Menu: "iam.user"}
perm.Def{Code: "iam:user:create", Name: "新增用户", Menu: "iam.user", Requires: []string{"iam:user:view"}}
perm.Def{Code: "iam:user:delete", Name: "删除用户", Menu: "iam.user", Requires: []string{"iam:user:view"}}
```

示例（用户管理页面）：

| 权限码 | 作用 | 超级管理员 | 运营 | 客服 | 审计 |
|---|---|---|---|---|---|
| iam:user:view | 显示菜单、进入页面、列表与详情 | ✓ | ✓ | ✓ | ✓ |
| iam:user:create | 新增用户 | ✓ | ✓ | | |
| iam:user:update | 编辑用户、启停 | ✓ | ✓ | ✓ | |
| iam:user:delete | 删除用户 | ✓ | | | |
| iam:user:reset-password | 重置密码 | ✓ | | ✓ | |
| iam:user:export | 导出用户 | ✓ | ✓ | | ✓ |

### 5.2 权限码命名

`{module}:{resource}:{action}`，例如 `iam:user:view`、`iam:role:grant`、`file:object:download`。

| action | 含义 |
|---|---|
| `view` | 页面访问 + 列表查询 + 详情。**每个页面菜单必须引用其资源的 `view` 码** |
| `create` `update` `delete` | 增改删 |
| `export` | 导出（需要单独授权，不随 `view` 附带） |
| `grant` `execute` 及其他 | 资源特有的操作，如 `reset-password` |

### 5.3 菜单可见性与权限依赖

- 菜单是否显示**只看 `view` 码**。用户只有 `iam:user:create`、没有 `iam:user:view` 时，菜单不显示。
- 为避免出现这种"有操作权限却进不了页面"的无效组合，操作权限通过 `Requires` 声明依赖，默认依赖本资源的 `view`。
- 依赖关系**由后端强制**：保存角色授权时，如果提交的权限集合不满足依赖闭包，接口返回 422 并列出缺失的权限码，不会隐式补全。
- 授权界面只做交互上的辅助：勾选 `create` 时自动勾选 `view`，取消 `view` 时一并取消依赖它的权限。最终写入数据库的仍是界面上显式勾选的集合，前后端规则一致。
- 依赖只在授权时校验，鉴权时不再推导，`sys_role_permissions` 中存的就是用户的全部有效权限码。

### 5.4 操作权限与数据权限相互独立

拥有 `iam:user:view` 只表示可以访问"用户"这类资源，**能看到哪些用户由数据权限 Scope 决定**（见 7.3）。两者在鉴权链路中依次生效：路由中间件检查权限码，Repository 查询施加 Scope。

### 5.5 超级管理员

- 内置角色 `super_admin`：不可删除、不可停用，权限不可编辑。
- 鉴权时直接放行；`/me` 返回**展开后的完整权限码列表**，前端无需特殊处理。
- 授予或撤销 `super_admin` 只能由超级管理员操作，并写入审计。
- 系统至少保留一个状态正常的超级管理员，最后一个不可移除。

### 5.6 鉴权与缓存

**缓存内容**：每个用户一份鉴权快照（Auth Snapshot），结构见 7.2。

```text
auth:snapshot:v{ver}:user:{id}  →  快照 JSON，TTL 10 分钟 ± 随机 60 秒
auth:ver                        →  当前全局版本号（PG 中版本号的镜像）
```

- **快照只是鉴权决策的缓存**，不是用户资料或组织信息的事实来源。PostgreSQL 是唯一事实来源，快照可以随时删除并重建。业务代码不得从快照读取用户资料用于展示或写入。
- 旧版本的快照不会被主动删除，依靠 TTL 自然过期。即使版本号频繁递增，Redis 中的快照数量也不超过"10 分钟内的活跃用户数 × 版本变更次数"。
- **防止缓存击穿**：版本号变化后，所有用户的快照同时失效。同一实例内对同一个 `(ver, user_id)` 的并发重建用 `singleflight` 合并；跨实例不加分布式锁，最多重复重建"实例数"次，每次只是几条走索引的查询，可以接受。

**每次请求**：Session → `user_id` → 读取 `auth:ver` → 读取快照，未命中则从 PG 构建后写入 → 判断权限码。会话、会话代数（见 6.5）和 `auth:ver` 用一次 pipeline 读取。

**版本号的一致性**

1. **事实来源在 PG**：单行表 `sys_auth_version`。所有影响鉴权的写操作（角色授权、用户角色、角色启停、用户启停、用户部门、数据范围配置）在**同一个数据库事务**内执行 `UPDATE sys_auth_version SET version = version + 1 RETURNING version`。
2. **事务提交后**同步到 Redis，并且只增不减（Lua 脚本：新值大于当前值才写入）。这样并发的多个写操作即使乱序到达，也不会把版本号回退。失败时重试 3 次。
3. **对账兜底**：每个实例每 10 秒比较一次 PG 与 Redis 的版本号，Redis 落后时同样单调写入。
4. **新旧快照不会混淆**：在旧版本号下用旧数据构建的快照，只会写进旧版本的键，而旧键在版本号推进后不再被读取。因此不会出现"新版本号的键里存着旧权限"的情况。
5. **故障关闭**：Redis 不可用时，会话和版本号都无法读取，请求返回 503，不会回退到进程内缓存或跳过鉴权。
6. **多实例一致**：P0 不使用进程内权限缓存，所有实例都读同一个 Redis，看到的版本号一致。如果以后为了性能引入本地缓存，键也必须带上从 Redis 读到的版本号。

**一致性语义（准确表述）**

权限变更是**有界延迟的最终一致**，不是严格实时：

- PG 提交之后、Redis 版本号更新之前，存在一个时间窗口，期间所有实例仍按旧快照鉴权。
- 正常情况下，这个窗口只是提交后同步 Redis 的一次网络往返（毫秒级）。
- Redis 同步失败时，依靠重试和对账收敛，**目标上界为 10 秒**。
- 所有实例都宕机、PG 不可用或对账持续失败时，不保证 10 秒上界。
- 因此，**停用账号、强制下线、修改密码不依赖这个机制**，它们走 6.5 的会话撤销路径，在操作返回成功时就已经生效。

**其他**

- 默认拒绝：注册表中找不到的路由一律 404，权限缺失返回 403。

### 5.7 「API 管理」页

只读展示注册表内容：方法、路径、访问级别、权限码、所属模块、模块是否启用。不建表，也不提供编辑。

---

## 6. 认证与会话

### 6.1 会话凭据

- 登录成功后生成 32 字节 `crypto/rand` 随机 token（base64url），写入 Cookie。
- Redis 中**以 `SHA-256(token)` 作为键**，即使 Redis 数据泄露，攻击者也拿不到可用的 token。
- 每次登录都签发新 token，并销毁请求中携带的旧会话（防止会话固定）。

```text
sess:{hash}            HASH  user_id, gen, created_at, last_seen_at, ip, user_agent, absolute_expire_at
                             TTL = 空闲超时
sess:gen:{user_id}     INT   会话代数的镜像（权威值在 sys_users.session_gen），不设 TTL，见 6.5
user_sess:{user_id}    SET   {hash...}            —— 用于清理会话键、在线会话列表
sess:active            ZSET  member=hash, score=last_seen_at —— 在线会话列表（P1 页面使用）
```

### 6.2 Cookie 属性

| 属性 | 值 |
|---|---|
| 名称 | 生产环境用 `__Host-sid`（可配置） |
| HttpOnly / Secure | 是 / 是（本地开发可关闭 Secure） |
| SameSite | `Lax` |
| Path / Domain | `/` / 不设置 |
| Max-Age | 不设置（会话 Cookie），服务端控制过期 |

### 6.3 过期与续期

- **空闲超时**（默认 2h，可配置）：请求到达时刷新 TTL。为减少写入，距离上次刷新超过 1 分钟才写回。
- **绝对超时**（默认 12h，可配置）：到期后必须重新登录，不能续期。
- 不再需要 `/auth/refresh` 接口，前端也不需要刷新令牌的逻辑。

### 6.4 CSRF

- `SameSite=Lax` 是第一层。
- 第二层：对 `POST/PUT/PATCH/DELETE` 请求校验 `Origin`（缺失时校验 `Referer`）必须属于配置的允许来源。
- 第三层：Double-submit token。登录时下发非 HttpOnly 的 `XSRF-TOKEN` Cookie，前端用 `X-XSRF-TOKEN` 请求头回传（同源下 axios 默认支持），服务端比对。

### 6.5 会话撤销

**目标**：停用账号、重置密码、强制下线在管理员的操作**返回成功时就已生效**，不依赖 5.6 的版本同步窗口。

#### 6.5.1 数据与不变量

| 存储 | 内容 | 角色 |
|---|---|---|
| PG `sys_users.session_gen` | 会话代数 | **权威值**，只增不减 |
| Redis `sess:gen:{uid}` | 代数镜像 | 请求期的快速检查。只能用"取较大值"的 Lua 脚本写入，**禁止 `DECR`，也禁止写入更小的值** |
| Redis `sess:{hash}.gen` | 会话创建时盖上的代数 | 创建后不再修改 |
| Auth Snapshot `SessionGen`、`Status` | 构建快照时从 PG 读取的值 | 第二道检查，Redis 数据回退时用来自愈 |

**不变量**

- I1：被撤销的会话，其代数严格小于撤销后的镜像值和 PG 值。
- I2：镜像缺失**不等于 0**，而是视为缓存未命中，从 PG 读取后用"取较大值"写回；PG 不可用时返回 503。
- I3：任何失败路径都只会让代数变大（多撤销一些会话），不会让代数变小（让会话复活）。

#### 6.5.2 请求期校验

```text
session 有效  ⇔  session.gen ≥ max(镜像代数, snapshot.SessionGen)  且  snapshot.Status = 启用
```

- 会话、镜像代数和 `auth:ver` 用一次 pipeline 读取。镜像缺失时按 I2 处理。
- 构建快照时，顺便把 PG 中的 `session_gen` 用"取较大值"写回镜像，用于修复镜像回退的情况。
- 撤销只对撤销完成**之后开始**的请求生效；撤销前已经通过校验、正在执行的请求不会被中断。

#### 6.5.3 撤销流程（停用、软删除、重置密码、强制下线全部会话通用）

```text
BEGIN
  SET LOCAL lock_timeout = '3s'
  SELECT session_gen FROM sys_users WHERE id = ? FOR UPDATE      → g
  Redis Lua: n = max(镜像, g) + 1; SET 镜像 = n; return n          ← 失败：ROLLBACK，返回 503，无任何副作用
  UPDATE sys_users SET session_gen = n, status/password_hash ...
  UPDATE sys_auth_version SET version = version + 1
COMMIT
→ 单调同步 auth:ver；异步清理会话键（只是清理，不影响正确性）
```

- **Redis 镜像先于 PG 提交写入，这是有意为之、不可回滚的安全副作用。** 只要镜像写入成功，旧会话就已经失效，与 PG 是否提交无关。
- 新代数取 `max(镜像, g) + 1` 而不是 `g + 1`，这样即使之前有一次提交失败、镜像领先于 PG，新代数也一定比所有已盖章的会话代数都大。
- 用户自己修改密码：走同一流程，提交后为当前客户端签发新会话（盖章代数为 `n`）。效果是其他设备下线、当前设备保持登录；如果提交失败，当前设备也会被登出。
- 强制下线单个会话：只删除该会话键，失败时接口返回错误。

#### 6.5.4 登录流程

```text
Argon2 校验密码（事务外，避免长时间持锁）
BEGIN
  SET LOCAL lock_timeout = '3s'
  SELECT status, password_hash, session_gen FROM sys_users WHERE id = ? FOR SHARE   → g
  复核：状态为启用，password_hash 与事务外校验时一致
  读取镜像 m（缺失时按 I2 处理）
  创建会话，盖章代数 = max(g, m)                                                     ← 在锁内完成
COMMIT
```

**共享锁覆盖了读取代数和创建会话的全过程**，与撤销流程的 `FOR UPDATE` 互斥。

#### 6.5.5 状态转换与对应测试

**场景 A：登录与停用并发**

| 顺序 | 过程 | 结果 | 测试 |
|---|---|---|---|
| 登录先拿到锁 | 登录盖章 c = max(g, m) 并提交 → 停用拿到锁，镜像改为 max(m, g) + 1 → 提交 | 刚创建的会话 c 小于新代数，失效 | A1 |
| 停用先拿到锁并提交 | 登录等待 → 读到停用状态 | 拒绝登录 | A2 |
| 停用先拿到锁、写入镜像 n，但提交失败 | 登录等待 → 读到启用状态、g、m=n → 盖章 n | 新会话有效，旧会话失效 | A3 |

**场景 B：PG 提交失败或结果未知**

| 情况 | 状态 | 处理 | 测试 |
|---|---|---|---|
| Lua 明确失败 | 没有任何变化 | 回滚，返回 503 | B1 |
| Lua 超时（可能已执行） | 镜像可能已增加，等同于 B2 | 回滚，返回 503，提示同 B2 | B1' |
| Lua 成功，提交明确失败 | 镜像 n > PG g，用户仍为启用，旧会话全部失效 | 返回错误："操作未完成，该用户现有会话可能已失效，请重试"。**不回退镜像** | B2 |
| B2 之后再次撤销 | 新代数 = max(n, g) + 1 = n + 1 | 所有会话（包括 B2 之后用 n 登录的会话）都失效 | B3 |
| 提交时连接断开，结果未知 | 可能已提交，也可能未提交 | 返回"结果未知，请刷新确认"。两种结果都满足 I1；已提交时 `auth:ver` 由对账同步。**验收**：接口返回专用错误码表示"结果未知"，不能当作失败处理；客户端通过查询用户状态确认最终结果；客户端重试只会再多递增一次代数，不会让安全状态回退 | B4 |

**场景 C：Redis 代数丢失或回退**

| 情况 | 结果 | 测试 |
|---|---|---|
| 镜像键缺失（重启且未持久化、误删） | 按 I2 从 PG 重新加载，不当作 0 处理 | C1 |
| 镜像回退到旧值（AOF 丢失最后一段写入） | 第二道检查 `snapshot.SessionGen` 生效：对账在 10 秒内修正 `auth:ver`，快照从 PG 重建，旧会话失效，镜像同时被修复。**这是唯一存在有界窗口的情况**。**验收**：必须验证对账确实能发现 Redis 与 PG 不一致并让旧快照失效；中间件比较的是会话盖章代数与可信的当前代数（镜像与重建后快照中的较大值），不能只依赖旧快照里的值 | C2 |
| Redis 整体丢失 | 会话也一起丢失，所有用户需要重新登录 | C3 |
| Redis 不可用 | 所有需要登录的请求返回 503 | C4 |
| PG 不可用、镜像缺失 | 返回 503，不会默认放行 | C5 |

#### 6.5.6 Redis 部署要求

- 会话、镜像和快照放在同一个 Redis 部署中。
- 淘汰策略只能是 `noeviction` 或 `volatile-*`，禁止 `allkeys-*`。启动时检查，不符合则拒绝启动；云 Redis 禁用 CONFIG 命令时改为配置项声明并告警。
- 建议开启 AOF（`appendfsync everysec`），以缩小场景 C2 的发生概率。**持久化只是降低风险，正确性靠的是 PG 权威值和不变量 I1–I3**，不依赖 Redis 持久化。

| 事件 | 处理 |
|---|---|
| 用户停用 / 软删除 / 管理员重置密码 / 强制下线全部会话 | 6.5.3 撤销流程 |
| 用户自己修改密码 | 6.5.3 撤销流程，然后签发新会话 |
| 强制下线单个会话 | 删除会话键 |
| 角色权限、部门、数据范围变更 | 不撤销会话，走 5.6 的版本号机制（有界延迟） |

### 6.6 登录防护（P0 基础版）

- 登录接口按 IP 和用户名两个维度在 Redis 中限流。
- 登录失败统一返回"账号或密码错误"，不暴露账号是否存在。
- 验证码、失败锁定、密码复杂度策略配置化放在 P1「安全策略」。

---

## 7. IAM 与数据权限

### 7.1 IAM 模块范围

`internal/modules/iam/` 下按子包组织：`user`、`role`、`dept`、`position`、`menu`、`permission`。对外只暴露一个 IAM Service 接口集合。

### 7.2 Auth Snapshot 与 Actor

```go
// 缓存在 Redis 中的鉴权快照（见 5.6），只用于鉴权决策
type AuthSnapshot struct {
    Version     int64         `json:"version"`     // 构建时的 auth 版本，读取时与键中的版本核对
    UserID      ID            `json:"userId"`      // ID 在 JSON 中序列化为字符串
    Status      UserStatus    `json:"status"`
    SessionGen  int64         `json:"sessionGen"`  // PG 中的 session_gen，会话校验的第二道检查（6.5.2）
    DeptID      ID            `json:"deptId"`
    IsSuper     bool          `json:"isSuper"`
    RoleIDs     []ID          `json:"roleIds"`     // 只包含启用状态的角色
    Permissions []string      `json:"permissions"` // 已排序；加载后在内存中转为 set
    DataScope   ResolvedScope `json:"dataScope"`   // 已合并多角色，见 7.3
}

// 单次请求的上下文，由中间件从 Session + Snapshot 构造
type Actor struct {
    Snapshot  *AuthSnapshot
    SessionID string
    RequestID string
    IP        string
}
```

由鉴权中间件构造并注入 `context.Context`，Service 和 Repository 通过 `actor.From(ctx)` 获取。需要用户资料（昵称、头像等）时，必须从 IAM Service 查询，不能读快照。

### 7.3 数据权限 Scope 接口（P0 定义，P1 实现策略）

```go
// 资源声明自己的归属字段
type ScopeSpec struct {
    DeptColumn  string // 如 "dept_id"，为空表示该资源不按部门隔离
    OwnerColumn string // 如 "created_by"
}

// Repository 的所有列表、详情、更新、删除、导出查询都必须经过它
func ApplyScope(ctx context.Context, db *gorm.DB, spec ScopeSpec) *gorm.DB
```

| 策略 | 条件 | 阶段 |
|---|---|---|
| 全部 | 不加条件 | P0 |
| 本人 | `owner = actor.UserID` | P1 |
| 本部门 | `dept_id = actor.DeptID` | P1 |
| 本部门及下级 | `dept_id IN (SELECT id FROM sys_departments WHERE path LIKE :prefix)` | P1 |
| 自定义部门 | `dept_id IN (sys_role_depts 中配置的部门)` | P1 |

**多角色合并语义（P0 确定，P1 实现 SQL）**

```go
type ResolvedScope struct {
    All         bool     `json:"all"`         // 任一角色为「全部」时为 true，其余字段忽略
    Self        bool     `json:"self"`        // 包含「本人」
    DeptIDs     []ID     `json:"deptIds"`     // 「本部门」与「自定义部门」的并集
    DeptSubtree []string `json:"deptSubtree"` // 「本部门及下级」的部门 path 前缀
}
```

- 多角色取**并集**：最终条件是 `owner = 我 OR dept_id IN (DeptIDs) OR 部门 path 以 DeptSubtree 之一为前缀`，三部分都为空时等价于 `false`（看不到任何数据）。
- **v1 不支持拒绝规则**。引入"拒绝优先"后，管理员要同时推理允许和拒绝两套规则，并集语义也会失效。确实需要排除某些数据时，通过收窄角色的允许范围来实现。
- 新建角色的数据范围默认为「本人」，不默认「全部」，与"默认拒绝"的原则保持一致。
- 停用的角色不参与合并；超级管理员的结果固定为 `All`。
- 用户没有任何角色（或角色全部停用）时，合并结果为空，看不到任何业务数据；同时也没有任何权限码。
- 部门移动、用户换部门、角色数据范围变更都会递增鉴权版本号（5.6），快照中的合并结果随之重建。

**规则**

- 数据范围是角色上的属性，与权限码相互独立：权限码决定能做什么操作，Scope 决定能作用在哪些数据上。
- **数据范围只收窄，不授权**：它不能授予任何操作权限。例如用户有 `iam:user:delete`、数据范围为「本人」，删除他人记录时，带 Scope 的加载查不到记录，返回 404。
- **配置粒度**：管理员在角色上配置一个数据范围，它作用于所有声明了 `ScopeSpec` 的资源。每个资源"按哪个字段隔离"由开发者在代码里声明，管理员不能按 API 单独配置。如果以后需要"同一角色对不同资源使用不同范围"，再扩展为"角色 × 资源"两级配置，接口 `ApplyScope(ctx, db, spec)` 不需要改动。
- **访问级别为 `Authenticated` 的接口同样要施加 Scope**。例如 `/users/options` 虽然不需要权限码，但只能返回 Scope 内的用户；`/departments/tree` 对部门资源本身施加 Scope（`DeptColumn = "id"`）。否则这类辅助接口就会成为绕过数据权限的通道。
- 详情、更新、删除操作必须先用带 Scope 的查询加载记录，加载不到时返回 **404**（不返回 403，避免泄露资源是否存在）。
- P0 阶段只实现「全部」，但 IAM 的所有查询都要接入 `ApplyScope`，并用测试保证这一点（例如用 mock scope 断言 SQL 中包含注入条件）。

### 7.4 部门树

- 物化路径 `path`，如 `/1/5/12/`，建立 `text_pattern_ops` 索引以支持前缀查询。
- 移动部门时在同一事务中更新所有子孙节点的 `path`。
- 校验：不能把部门移动到自身或其子孙节点下；存在子部门或用户时不能删除。

---

## 8. 数据库设计

### 8.1 通用约定

| 项 | 约定 |
|---|---|
| 主键 | `id bigint GENERATED ALWAYS AS IDENTITY` |
| JSON 中的 ID | 字符串。Go 中定义 `type ID int64`，自定义 `MarshalJSON` / `UnmarshalJSON` |
| 时间 | `timestamptz`，API 输出 RFC 3339 |
| 审计字段 | `created_at`, `updated_at`, `created_by`, `updated_by` |
| 乐观锁 | 可编辑实体带 `version int NOT NULL DEFAULT 0`；`UPDATE ... WHERE id=? AND version=?`，影响行数为 0 时返回 **409** |
| 软删除 | 只在需要的表上使用 `deleted_at`；唯一约束改为部分唯一索引 `WHERE deleted_at IS NULL` |
| 状态 | `status smallint`：1 启用，2 停用 |
| 大小写 | 用户名唯一索引建在 `lower(username)` 上 |

### 8.2 P0 表

| 表 | 关键字段 | 说明 |
|---|---|---|
| sys_users | username, password_hash, nickname, email, phone, avatar_url, dept_id, status, session_gen（只增不减，见 6.5）, must_change_password, password_changed_at, last_login_at, last_login_ip, version, deleted_at | 默认停用，必要时软删除 |
| sys_roles | code, name, data_scope_type（默认 self）, status, is_builtin, sort, remark, version, deleted_at | `code` 部分唯一 |
| sys_user_roles | user_id, role_id | 复合主键 |
| sys_departments | parent_id, path, name, leader_user_id, sort, status, version | |
| sys_positions | code, name, sort, status, version | 岗位与角色解耦 |
| sys_user_positions | user_id, position_id | |
| sys_permissions | code (PK, varchar(128)), module, name, menu_code, requires (text[]), status(active/deprecated), synced_at | 由启动同步写入 |
| sys_role_permissions | role_id, permission_code（FK → sys_permissions.code），PK(role_id, permission_code) | **授权的唯一事实来源** |
| sys_auth_version | id smallint PK（固定为 1）, version bigint | 鉴权版本号的事实来源，见 5.6 |
| sys_menus | code (唯一), parent_id, type(dir/menu/link), title, path, component, icon, sort, hidden, keep_alive, permission_code, external_url, source(module/custom), module, version | |

会话只存在 Redis，P0 不建 `sys_sessions` 表。

### 8.3 后续阶段的表

| 阶段 | 表 |
|---|---|
| P1 | sys_role_depts（自定义数据范围）、sys_dict_types、sys_dict_items、sys_configs、sys_config_changes、sys_login_logs、sys_operation_logs、sys_files |
| P2 | sys_jobs、sys_job_runs |

v1.0 中的 `sys_api_resources`、`sys_role_api_permissions`、`sys_access_audits`、`sys_error_logs`、`sys_codegen_records`、`sys_plugins`、`sys_sessions` 均**取消**。

---

## 9. API 规范

- 统一前缀 `/api/v1`，资源化 REST 风格。
- 响应信封：

```json
{ "code": 0, "message": "success", "data": { "id": "1001" }, "requestId": "req_..." }
```

- 分页：请求参数为 `page`、`pageSize`（上限 100）；响应 `data` 为 `{ list, total, page, pageSize }`。
- 业务错误码按模块分段：`1xxxx` 通用、`2xxxx` 认证、`3xxxx` IAM，后续模块依次分配。
- HTTP 状态码语义：

| 状态 | 场景 |
|---|---|
| 401 | 未登录、会话过期或已撤销 |
| 403 | 已登录但缺少权限码；CSRF 校验失败 |
| 404 | 资源不存在，或不在数据范围内 |
| 409 | 版本冲突、唯一键冲突 |
| 422 | 参数校验失败（附字段级错误） |
| 429 | 被限流 |

- 错误响应中不得出现 SQL、堆栈、token 或配置值；详细信息只写入 Zap 日志，通过 requestId 关联。

### 9.1 P0 接口清单

| 方法 | 路径 | 访问级别 |
|---|---|---|
| POST | /auth/login | Public（限流） |
| POST | /auth/logout | Authenticated |
| GET | /me | Authenticated（返回用户信息、角色、权限码列表） |
| GET | /me/menus | Authenticated（pure-admin 动态路由格式） |
| PUT | /me/password | Authenticated |
| PUT | /me/profile | Authenticated |
| GET | /users, /users/:id | iam:user:view |
| POST / PUT / DELETE | /users, /users/:id | iam:user:create / update / delete |
| GET | /users/export | iam:user:export |
| PUT | /users/:id/status | iam:user:update |
| PUT | /users/:id/password | iam:user:reset-password |
| GET | /users/options | Authenticated |
| GET/POST/PUT/DELETE | /roles[/:id] | iam:role:* |
| GET/PUT | /roles/:id/permissions | iam:role:grant（PUT 校验依赖闭包，见 5.3） |
| GET/POST/PUT/DELETE | /departments[/:id] | iam:dept:* |
| GET | /departments/tree | AnyPerm(iam:dept:view, iam:user:view) |
| GET/POST/PUT/DELETE | /positions[/:id] | iam:position:* |
| GET/POST/PUT/DELETE | /menus[/:id] | iam:menu:* |
| GET | /permissions/tree | iam:role:grant（授权页使用） |
| GET | /api-routes | iam:api:view（只读） |
| GET | /health/live, /health/ready | Public（不带 /api/v1 前缀） |
| GET | /metrics | 仅内网或独立端口 |

---

## 10. 前端对接（pure-admin-thin）

1. 锁定 pure-admin-thin 的某个 tag/commit，记录到 `web/UPSTREAM.md`，并核对许可证。
2. Vite proxy 将 `/api` 代理到后端，保证开发期同源。
3. **改造认证**：删除模板中的 token 存取与刷新逻辑（`utils/auth.ts`、http 拦截器中的过期判断和刷新流程，具体以锁定版本的代码为准）。axios 启用 `withCredentials`，并配置 XSRF Cookie 和请求头名称。
4. 收到 401 时清空本地用户状态，跳转登录页并带上 redirect 参数；收到 403 时提示"无权限"，不跳转。
5. `localStorage` 只保存非敏感的展示信息（昵称、头像），不保存任何凭据。
6. `/me/menus` 直接返回模板动态路由需要的结构，前端不做二次映射。
7. **按钮权限统一入口**：`src/utils/permission.ts` 导出 `hasPerm(code)`，并注册 `v-perm` 指令；业务页面只使用这两者。如果锁定版本的模板已经提供等价能力（如 `hasPerms` / `v-perms`），就由它们包装实现，不维护两套。权限码列表来自 `/me`，存放在 Pinia 中，用 `Set` 判断。

   ```vue
   <el-button v-perm="'iam:user:create'" type="primary" @click="createUser">新增用户</el-button>
   <el-button v-perm="'iam:user:delete'" type="danger" @click="remove(row)">删除</el-button>
   ```

   前端隐藏按钮只是为了体验，安全边界在后端：手工构造 `DELETE /users/123` 的请求，没有 `iam:user:delete` 权限同样会收到 403。
   页面路由：`/me/menus` 只返回有 `view` 权限的菜单，未授权页面不会被注册为路由，直接输入 URL 会进入 404 页面。
8. 每个模块建立 `src/api/<module>/*.ts` 和 `src/views/<module>/*`；封装通用的分页表格、搜索表单、ID 字符串处理和 409 冲突提示（"数据已被他人修改，请刷新"）。
9. 边界场景：刷新页面后恢复菜单、权限变更后重新拉取 `/me`、会话被踢后的提示文案。

---

## 11. 工程目录

```text
base-admin/
├── server/
│   ├── cmd/
│   │   ├── admin/main.go          # HTTP 服务
│   │   └── gen/                   # 代码生成 CLI（P2）
│   ├── internal/
│   │   ├── app/                   # 组装：配置、依赖、模块列表、启动流程
│   │   ├── core/                  # 框架核心，下游尽量不改
│   │   │   ├── module/            # Module 接口、注册表、启动校验
│   │   │   ├── perm/              # 权限码定义、鉴权、缓存
│   │   │   ├── menu/              # 菜单定义与同步
│   │   │   ├── actor/             # Actor 与 context
│   │   │   └── datascope/         # Scope 接口与策略
│   │   ├── platform/              # 基础设施适配
│   │   │   ├── config/ database/ redis/ session/
│   │   │   ├── httpx/             # Router 包装、响应、错误、分页
│   │   │   ├── middleware/
│   │   │   └── observability/     # Zap、metrics、health
│   │   └── modules/
│   │       ├── auth/              # 登录、登出、/me
│   │       ├── iam/               # user role dept position menu permission
│   │       ├── system/            # 字典、参数、基础设置（P1）
│   │       ├── audit/             # 登录日志、操作审计（P1）
│   │       ├── file/              # 文件与存储（P1）
│   │       └── job/               # 定时任务（P2）
│   ├── migrations/
│   ├── configs/
│   └── tests/                     # 集成测试、越权测试
├── web/                           # pure-admin-thin
├── deploy/ (docker, nginx)
└── docs/
```

每个模块内部按 `handler / service / repository / model / dto / routes.go / module.go` 组织。

---

## 12. 安全与非功能要求

在 v1.0 第 10 节的基础上，补充和修订如下：

- 密码：Argon2id，参数可配置并随哈希一起存储，以便后续升级参数。
- 会话凭据：只出现在 HttpOnly Cookie 中，日志中出现 Cookie 和 Authorization 头时一律脱敏。
- 响应头：`X-Content-Type-Options`、`X-Frame-Options`/CSP `frame-ancestors`、`Referrer-Policy`、HSTS（由 Nginx 设置）。
- 请求体大小上限、请求超时、优雅退出（先摘流量，再等待处理中的请求，最后关闭连接池）。
- `/health/ready` 检查 PG 和 Redis 连通性，优雅退出期间返回 503；`/health/live` 只反映进程存活。
- 迁移：生产环境由独立命令执行，不在服务启动时自动执行。

---

## 13. 测试策略

| 类型 | 内容 |
|---|---|
| 单元 | 权限码校验规则、启动校验各条规则、权限依赖闭包校验、Scope 合并、乐观锁、密码哈希、ID 序列化 |
| 鉴权缓存 | 版本号单调写入（乱序到达不回退）；提交后 Redis 同步失败时，对账在一个周期内收敛；Redis 不可用时返回 503；版本变更后并发请求只触发一次重建（singleflight） |
| 会话撤销 | 6.5.5 中 A1–A3、B1–B4、C1–C5 每个场景至少一个用例（并发用例用两个事务配合屏障控制执行顺序；故障用例对 Redis/PG 注入错误）；修改密码后其他会话失效、当前会话保持 |
| 数据范围 | 多角色合并结果；无角色时结果为空；「本人」范围下删除他人记录返回 404 |
| 集成（`integration` 标签） | 登录/登出/过期/绝对超时、停用后会话立即失效、改密撤销其他会话、部门移动更新 path、软删除后用户名可复用 |
| **自动越权测试** | 遍历路由注册表：每个非 Public 路由在无会话时必须返回 401，每个 Perm 路由在无对应权限时必须返回 403。新增路由自动纳入，无需手写用例 |
| CSRF | 缺少或错误的 XSRF token、跨源 Origin 的写请求都返回 403 |
| 前端 E2E（Playwright） | 登录、动态菜单、按钮显隐、刷新恢复、被踢下线 |

覆盖率目标 80%，`core` 与 `auth` 包要求更高。

---

## 14. 里程碑

### 阶段 1（P0）：最小可用后台

1. **工程骨架**：Gin、GORM、PostgreSQL、Redis、配置、migrations、统一响应与错误、Zap、health/metrics、Docker Compose 开发环境、CI（lint、test）。
2. **模块注册**：Module 接口、httpx.Router 访问级别、启动校验、权限码和菜单同步、幂等种子数据。
3. **认证会话**：登录、登出、空闲/绝对过期、强制下线、停用即失效、CSRF、登录限流。
4. **IAM**：用户、角色、部门、岗位、菜单、权限授权，以及 DataScope 扩展点（仅实现「全部」）。
5. **前端对接**：pure-admin-thin 改造、登录、动态菜单、按钮权限、IAM 管理页面。

**验收标准**

- 普通管理员登录后只能看到并调用被授权的菜单和接口；越权测试对全部路由通过。
- 停用账号的接口返回成功后，该账号已登录的浏览器在下一次请求时收到 401；Redis 不可用时停用操作失败，不会出现"显示已停用但会话仍有效"。
- 修改角色权限后，正常情况下相关用户的下一次请求就按新权限鉴权，无需重新登录；Redis 同步失败时，目标在 10 秒内收敛。
- 同一个用户管理页面，按 5.1 示例配置的四个角色看到的按钮各不相同；提交缺少 `view` 依赖的授权会被拒绝（422）。
- 两人同时编辑同一用户，后提交者收到 409 并在界面上看到提示。
- 故意在代码中引用未声明的权限码时，服务拒绝启动并给出明确错误。
- 刷新页面后菜单和权限正确恢复。

### 阶段 2（P1）：系统基础能力

- 数据权限策略：本人、本部门、本部门及下级、自定义部门
- 数据字典、系统参数（含变更记录）、基础设置
- 安全策略：密码复杂度、验证码、失败锁定、首次登录强制改密
- 在线会话页面
- 登录日志、操作审计（写操作、敏感读取、导出、授权变更），以及保留期限和定期清理
- 文件：本地 / S3 兼容存储驱动、上传校验、私有文件短时签名 URL、媒体库

**验收**：部门级数据隔离在列表、详情、导出接口上都生效；敏感操作可追溯；私有文件无法越权下载。

### 阶段 3（P2）：运维与开发效率

- 定时任务：仅允许预注册的 handler；多实例下用 Redis 锁防止重复执行，支持超时、重试，记录执行日志
- 代码生成 CLI：读取 PG 表结构，生成后端模块（含 module.go、权限码、菜单、迁移草案、测试骨架）和前端 api/views；默认不覆盖已存在的文件，需显式加 `--force`
- OpenAPI 文档（swaggo）

**验收**：从一张表生成的模块可以直接编译运行，并且自动纳入越权测试。

### 阶段 4（P3）：模板化交付

- ~~示例业务模块（展示模块注册、数据权限、审计的完整用法）~~（已取消，见 §18；改由代码生成器和 module-guide.md 说明）
- 「如何新增一个模块」文档、下游项目初始化脚本（替换项目名、模块路径）
- 部署脚本、生产配置示例、运维手册

**验收**：按文档新增一个独立业务模块时，不需要修改 `core`、`platform`、`auth`、`iam` 的任何代码。

---

## 15. 仍待确认

| 项 | 说明 |
|---|---|
| 团队规模与时间预期 | 决定阶段 1 的拆分粒度和验收节奏 |
| 规模与性能目标 | 用户数、并发、P99 延迟，用于确定连接池、缓存和日志保留策略 |
| 部署形态 | Docker Compose 还是 K8s；影响 health 探针、配置注入、迁移执行方式 |
| 国际化 | 是否需要多语言；影响菜单标题和错误信息的存储方式 |
| 二次验证机制 | v1.0 中"敏感操作需二次验证"：重新输入密码，还是 TOTP |
| 表单配置 | v1.0 中的"表单配置"是指运行时动态表单，还是代码生成器的字段配置？v2.0 暂按后者归入 CLI |
| 会话超时默认值 | 空闲 2h / 绝对 12h 是否符合使用习惯 |
| 数据范围粒度 | v1 按角色统一配置；是否已有明确场景需要"同一角色对不同资源使用不同范围" |

---

## 16. P0 冻结约束与实施顺序

以下约束在 P0 期间冻结。需要修改时，先更新本文档并说明原因，再改代码。

| 项目 | 约束 |
|---|---|
| 事实来源 | PostgreSQL 是唯一的持久化事实来源；Redis 只存会话、镜像和缓存 |
| 会话代数 | PG 中的值为权威值；镜像只能单调递增；缺失时从 PG 加载，不当作 0 |
| 登录与撤销 | 用同一用户行的锁协调（`FOR SHARE` / `FOR UPDATE`），锁覆盖 Redis 读写 |
| 权限版本 | 在 PG 事务内递增，提交后单调同步到 Redis，并定期对账 |
| 授权模型 | 权限码 + `sys_role_permissions`；页面访问统一用 `view`；依赖由后端校验；不引入 Casbin 或 ABAC |
| 数据权限 | P0 实现 Scope 接口与「全部」策略，P1 扩展其他策略；多角色取并集，不支持拒绝规则 |
| 模块注册 | 编译期显式注册；任意启动校验失败都拒绝启动 |
| API 注册 | 必须声明访问级别；禁止绕过统一注册器 |
| 前端 | pure-admin-thin，锁定版本 |
| 自动测试 | 覆盖 401、403、权限依赖、6.5.5 全部场景 |

**实施顺序**

1. **工程骨架**：配置加载、PG、Redis、Zap、migrations、HTTP Server、health/metrics、统一响应与错误、Docker Compose、CI。
2. **模块注册**：Module 接口、`httpx.Router` 访问级别、权限码与菜单注册、种子数据、启动校验。
3. **认证会话**：Opaque Token、Redis Session、会话代数（先写 6.5.5 的测试，再实现）、Cookie、CSRF、登录限流。
4. **IAM**：用户、角色、部门、岗位、菜单、权限依赖、Auth Snapshot、数据 Scope 接口。
5. **前端与测试**：登录、动态菜单、按钮权限、角色授权页、自动越权测试、E2E。

---

## 17. P0 实施记录：偏差与补充

P0 实施过程中对本文的偏差与补充记录在这里，每条都写明原因。第 16 节的冻结约束中，只有第一条被修改。

| # | 本文原内容 | 实际实现 | 原因 |
|---|---|---|---|
| 1 | 7.3 / 16：P0 只实现「全部」策略，其他策略留到 P1 | **五种策略全部在 P0 实现**，`sys_role_depts` 表也一并提前到 P0 | 7.3 规定新建角色默认是「本人」。如果 P0 只实现「全部」，这个默认值就无法生效，两条规定互相矛盾。另外各策略的 SQL 只是对同一个接口的少量补充，提前实现几乎不增加成本 |
| 2 | 4.2：路由包装器叫 `httpx.Router` | 实际是 `core/module.Router` | 包装器依赖 `core/access`，而平台层不能依赖核心层 |
| 3 | 3：模块之间只能通过对方的 Service 访问数据 | **约定的例外**：auth 模块负责 `sys_users` 中与认证相关的列（`password_hash`、`session_gen`，以及登录时的状态检查），并且为构建快照只读访问 IAM 的表 | 会话协议（6.5）要求在同一事务中锁定用户行并读写这些列，如果通过接口间接访问，就无法满足 6.5.4 的加锁要求 |
| 4 | 8.2：角色带 `deleted_at` 字段 | 角色和岗位改为**硬删除**；已分配给用户的不能删除（返回 30003） | 软删除会带来编码唯一性和关联清理问题，而硬删除前已检查过没有引用 |
| 5 | 未规定 | **防越权总规则（outranks）**：非超级管理员只能管理这样的用户：对方的有效权限码和数据范围都是自己的子集。这条规则在修改前和修改后（同一事务内）各校验一次，覆盖分配角色、扩大角色范围、调动部门、修改自己的信息、重置更高权限者的密码等所有路径。同时还要求：修改角色时，该角色的持有人修改后仍然不能超过调用者；不能管理权限超出自己的角色；自定义数据范围的部门必须在自己的范围内；不能授予自己没有的权限码；只有超级管理员能修改超级管理员；不能停用或删除自己；最后一个可用的超级管理员不能被移除 | 统一权限码以后，`iam:role:*` 和 `iam:user:*` 本身就是提权通道。安全评审发现，逐项属性检查有 3 个高危漏洞，因此改为这一条总规则（见下方第 12–16 条） |
| 6 | 未规定 | `Authenticated` 级别的辅助接口（如 `/users/options`、`/departments/tree`）同样受数据范围过滤；越界访问统一返回 404 | 这是 7.3 的要求，这里明确落到了实现上 |
| 7 | 未规定 | 管理员重置密码后，该用户下次登录必须修改密码；首个管理员由 `ADMIN_BOOTSTRAP_ADMIN_PASSWORD` 创建，首次登录也必须修改密码 | 4.4 只写了"首次登录强制修改" |
| 8 | 6.5.2：一次 pipeline 读取会话、代数和版本号 | 改为两次往返：先读会话，再用一个 pipeline 读代数和版本号 | 读代数需要会话里的 user_id，只读一次做不到 |
| 9 | 未规定 | 数据库触发器保证 `session_gen` 只增不减 | 在数据库层强制不变量 I3 |
| 10 | 未规定 | 导出 CSV 时，以 `= + - @` 开头的单元格前加 `'` | 防止表格公式注入 |
| 11 | 6.1 | Redis key 没有前缀，要求**每个部署独占一个 Redis 逻辑库**；集成测试按包分配不同的逻辑库 | 不同测试库的用户 ID 会重复，共用 Redis key 会互相污染。生产环境同理 |
| 12 | 6.6 | 登录限流改为**先预占**：在校验密码之前，用 Lua 原子计数，超出额度直接拒绝。Argon2 的并发数用信号量限制 | 原来先比对后计数，并发突发可以绕过额度（评审实测 150 个并发请求中有 33 个被处理）；每次 Argon2 计算占用 19 MiB 内存，可以被用来打满内存 |
| 13 | 未规定 | 停用、删除用户时，版本校验放在 `Prepare` 阶段，也就是提升会话代数之前 | 用一个过期的 version 就能让对方的会话全部失效，却不改动任何数据 |
| 14 | 7.4 | 部门的新增、移动、删除在 advisory lock 下串行执行 | 两个部门互相移动到对方之下，可能形成循环，并让物化路径损坏 |
| 15 | 未规定 | `/users/options` 只按用户名和昵称匹配；`/roles/options` 对非超级管理员隐藏 `super_admin`；`/me/password` 也限流；管理员新建的用户首次登录必须修改密码；API 响应加 `Cache-Control: no-store`；头像地址只允许 http(s) 或站内路径；id 列表限制长度 | 安全评审的中低危项 |
| 16 | 未规定 | 生产环境强制：Cookie 名使用 `__Host-` 前缀，`allowed_origins` 中不能包含 localhost | 防止生产环境沿用开发配置 |

**已知的剩余风险（P1 处理）**

- 按账号限流可以被用来锁定账号：攻击者连续输错密码，真实用户在窗口期内也会被拒绝。可以考虑按 IP+账号分别计数，或对已知设备豁免。
- 按 IP 限流时，IPv6 地址没有按 /64 聚合。
- 除登录和改密以外的接口还没有通用限流。
- Redis 连接不支持 TLS。
- `app.env` 默认值是 `dev`，生产部署必须显式设置。


**测试对应关系**

- 6.5.5 的 A1–A3、B1、B1′、B2/B3、B4、C1–C5：`server/internal/modules/auth/revocation_integration_test.go`。测试中关闭了会话 key 清理，确保 401 来自代数校验，而不是 key 被删除。
- 第 13 节的自动越权测试：`server/internal/app/iam_integration_test.go` 中的 `TestEveryRouteIsProtected`。
- 安全评审各项发现的回归测试：`server/internal/app/security_integration_test.go`（H1–H3、M1、M2、M4、L1）、`server/internal/modules/auth/http_integration_test.go`（M3 突发、改密限流）。每个测试都做过变异验证：去掉对应修复后测试会失败。

## 18. P1–P3 实施记录：偏差与补充

P1–P3 按 [p1-p3-plan.md](p1-p3-plan.md) 交付：审计、系统配置、安全中心、文件、运维、开发工具、工作台、部署文件、初始化脚本。与计划的差异如下：

| # | 计划 | 实际 | 原因 |
|---|---|---|---|
| 1 | P0 的「接口列表」页（`iam.api`，`iam:api:view`） | 由开发工具 →「接口文档」替代；`iam:api:view` 同步时标记为 deprecated，已有授权保留但不再生效 | 同一份路由注册表，接口文档额外提供参数、约束、示例和 OpenAPI JSON |
| 2 | 路由只声明访问级别 | 路由可链式声明 `.Doc()`、`.MaxBody(n)`、`.Timeout(d)`。`Timeout` 大于默认值时，同时用 `http.ResponseController` 延长该连接的读写截止时间 | 大文件上传和下载会超过全局的 `read_timeout`、`write_timeout`、`request_timeout`，其他路由仍使用默认值 |
| 3 | 定时任务使用 robfig/cron | 只复用其解析器，调度循环自己实现。每次触发以计划时间为键，用 `SET NX` 抢占（保证多实例只执行一次），执行期间持有任务锁（Lua 比较后删除）；`@every` 对齐到间隔的整数倍；进程崩溃留下的 running 记录按 `expires_at` 修正 | 多实例下需要知道每次触发的确切计划时间，才能去重 |
| 4 | 内置任务由 `Seed` 写入 | 内置任务由迁移插入一次（000009 清理执行记录、000010 清理审计日志、000011 清理已删除文件），管理员可以修改或删除，删除后不会被重新创建 | `Seed` 每次启动都会运行，会把管理员删掉的任务又加回来 |
| 5 | 审计清理天数写在任务参数中 | 参数为空时使用「基础设置 → 日志保留天数」，参数里写了 `days` 则以参数为准。审计模块通过 `Deps.RetentionDays` 函数取值，不依赖 system 包 | 保证设置页上的保留天数真正生效 |
| 6 | 文件表按计划设计 | 增加 `kind`、`version` 列；增加 `GET /files/upload-policy`；下载支持 `?download=1`；html、svg、js、可执行文件等**始终拒绝**，基础设置里也无法放开；删除时软删除记录和删除对象在同一事务中完成，存储删除失败返回 503 并保留记录 | 按类型筛选、乐观锁、前端预校验；防止存储型 XSS；保证删除可以重试 |
| 7 | 未规定 | 文件**不按数据范围过滤**：拥有 `file:file:view` 的用户可以看到整个媒体库 | 媒体库是共享资源；需要按部门隔离的项目可以在 `file/files.go` 的 `liveFiles` 中加上 `actor.Scope`（`OwnerColumn: "f.uploaded_by"`） |
| 8 | 未规定 | 登录失败计数按**规范化后的用户名**（去空格、小写）计算并用于触发验证码，硬锁定按「账号 + IP」计算（见下方评审处理）；管理员可以手动解锁；验证码一次性使用（GETDEL），2 分钟过期 | P0 剩余风险中的「按账号锁定」问题，改由策略控制（阈值可设为 0 关闭）并提供手动解锁 |
| 9 | 未规定 | 在线会话列表依赖 Redis 有序集合 `sess:active`，列表接口顺带清理已过期的会话 | 不能用 `SCAN` 遍历全部会话 key |
| 10 | 交付一个示例业务模块（客户管理） | **取消**：示例模块实现后又删除，迁移顺延（审计清理任务 000010、文件清理任务 000011）。生成器支持字典字段等改进写在 `module-guide.md` 里 | 业务模块不属于基础后台的范围 |
| 11 | 未规定 | 内置菜单使用的 Remix 图标预先注册在 `builtinIcons.ts`，内网部署时无需访问 Iconify API | 原来菜单图标需要在线加载 |
| 12 | 未规定 | 构建时注入版本号（`buildinfo.Version`，Docker 构建参数 `VERSION`），实例 ID 为「主机名-PID」 | 服务器监控页和执行记录需要显示版本和实例 |
| 13 | `scripts/new-project.sh` 只改 Go import | 改为替换所有提到模块路径的文本文件（包括生成器模板、Dockerfile 的 `-ldflags -X`、`.golangci.yml`），最后运行 build 和 `vet -tags integration` 验证 | 实测发现，只改 import 的话，改名后生成器产出的代码无法编译，版本号注入也会悄悄失效 |

**代码生成补充（实际试用后的改进）**

在临时项目副本中，用独立数据库完整走了一遍「建表 → 页面配置 → 生成到项目 → 重启 → 页面增删改查与搜索」。走查中发现并修复了以下问题：

| 问题 | 处理 |
|---|---|
| 中文名只能取列注释，搜索条件由生成器决定（几个文本列合成一个关键字框） | 增加字段配置：每个字段可设置中文名、是否在列表显示、是否在表单中、搜索方式（不搜索 / 模糊 / 精确 / 范围，按类型限制可选项）。每个搜索字段生成独立的搜索控件和后端条件；生成的集成测试会覆盖每个搜索参数。命令行用 `-fields '<JSON>'` 传入同样的配置 |
| 只能预览，生成要复制命令到本地执行，接入模块要手改代码 | 增加「生成到项目」（仅开发环境；不加覆盖就不改已有文件）。新模块插入 `modules.go` 的 `// codegen:modules` 标记；在已有模块中新增资源时插入该模块 `module.go` 的 `codegen:permissions/menus/routes` 标记。新建页面文件后 Vite 会刷新整个页面，页面会取回结果并恢复表单 |
| `numeric NOT NULL DEFAULT 0` 这类字段不出现在表单中 | 只有函数默认值（`now()`、`gen_random_uuid()` 等）才交给数据库；字面量默认值的字段可以编辑，表单预填默认值 |
| 唯一列重复时返回 409「数据已被他人修改」，对话框还被关闭 | 生成的代码把唯一约束冲突转成该字段的 422 错误（「订单号已存在」）；409 只表示并发修改 |
| 目录下只有一个页面时，目录被折叠，用户填写的模块标题看不到 | `/me/menus` 给目录下的菜单加上 `showParent`，目录始终显示 |
| 布尔列标题换行；删除确认显示内部 ID；导出写部门 ID | 标签列宽度按标题长度计算；删除确认显示唯一文本列（如订单号）；导出写部门名称 |
| 代码生成页的占位符仍是「客户」示例 | 改为订单示例 |
| 业务下拉选项写死在生成的代码里 | 字段配置增加「字典」列：可编辑的文本、整数字段可以绑定数据字典。表单和搜索用字典下拉，列表按字典标签显示，后端保存时校验取值必须是启用的字典项；绑定后自动显示在列表、只能精确搜索。系统内置的下拉（启用/停用、数据范围、验证码模式）的取值有代码含义，保持固定 |
| PostgreSQL 常用 `text` 存标题，默认被当成长文本不显示在列表 | 非空的 `text` 字段默认显示；可为空的 `text`（通常是备注）默认隐藏 |

**基础功能补充（对照 gin-vue-admin、若依等常见后台）**

| 功能 | 说明 |
|---|---|
| 通知公告 + 我的消息 | 管理端由代码生成器生成后扩展：草稿 / 发布 / 撤回，类型使用字典 `notice_type`；所有用户在顶栏铃铛看到未读数，「我的消息」可查看详情、全部已读。内容为纯文本，换行原样显示 |
| 用户导入 | 下载 CSV 模板，填写后上传并统一设置初始密码（首次登录须修改）；支持 Excel 另存的 GBK 编码；部门填名称或路径，角色、岗位填编码或名称；任一行有误则整批不导入并逐行提示，每次最多 1000 行 |
| 菜单图标选择 | 菜单表单改为从约 100 个常用图标中选择（分组 + 搜索），图标数据离线打包（`web/scripts/gen-menu-icons.mjs` 生成），内网部署不访问外网 |
| 下拉选项来源 | 代码生成的「下拉选项」可选：不使用 / 字段注释中的选项（注释写成「类型：1=通知,2=公告」，自动识别并默认使用、默认精确搜索，选项生成到代码里，后端用 oneof 校验）/ 数据字典（运行时可维护） |
| 建表规范 | 新增 `docs/table-guide.md`：必须字段 id、标准字段 version/created_at/updated_at/created_by/updated_by、约定字段 status/sort/dept_id/deleted_at、类型与控件、注释写法、唯一约束。`make new-migration NAME=create_<表>` 按规范生成建表模板；生成预览时提示缺少的标准字段 |
| 操作日志模块名 | 原来是前端写死的中文名表，新模块只显示英文标识；改为后端按各模块顶层目录菜单的标题返回，生成的模块自动显示中文，菜单改名后同步 |
| 业务附件 | `POST /files/attachments` 登录即可上传（规则同基础设置的上传规则，`?kind=image` 只收图片）；返回的地址 `/api/v1/files/a/<日期>/<128 位随机串>.<扩展名>` 只要求登录即可访问，不需要文件管理权限，也无法按 ID 遍历。前端组件 `ReImageUpload`（单图，存地址）、`ReFileUpload`（多附件，存 jsonb）；后端用 `internal/core/attachment` 校验，只允许保存附件地址。个人中心头像改为上传，通知公告支持附件 |
| 代码生成上传字段 | 字段配置的「下拉选项」列改为「控件」列：图片上传（长度 ≥ 255 的文本字段）、附件上传（jsonb 字段）、注释选项、数据字典。按字段名或注释自动识别（cover_url、封面；attachments、附件）。生成的后端用 `internal/core/attachment` 校验地址，表单用上传组件，列表显示缩略图或附件个数 |
| 关联字段 | 读表结构时识别外键（指向另一张表 id 的单列外键）；生成的表单为可搜索的远程下拉（`ReRemoteSelect`），列表、详情、导出显示名称，搜索可按关联记录精确筛选；后端新增 `/options/<字段>` 接口并在保存时校验记录存在（`internal/core/lookup`，目标表有 deleted_at 时排除已删除记录）。代码生成页的「控件」列可选择显示列或关闭 |
| 未做 | 表单设计器、插件市场、多语言等不属于基础后台的常用范围 |

**工程化补充（按外部评审修正）**

| 问题 | 处理 |
|---|---|
| CI 的前端 lint 会自动修改代码（`--fix`/`--write`），错误被悄悄修掉 | `pnpm lint` 只检查（prettier `--check`），`pnpm lint:fix` 本地修复；CI 构建后 `git diff --exit-code`。顺带修掉一处 stylelint 与 prettier 互相改写的行内样式 |
| `config.example.yaml` 缺 `auth.*`、`bootstrap.*` | 补全并注明生产配置；测试保证示例覆盖全部非密钥配置项且取值等于内置默认值 |
| E2E 未进 CI | 新增 e2e 任务：全新数据库、初始密码启动，E2E 初始化时自动完成首次改密 |
| nginx 长缓存写在 `/assets/`，实际产物在 `/static/`；location 内 `add_header` 让安全头丢失 | 改为 `/static/` 一年缓存，各 location 显式带安全头，新增 `/healthz` |
| 生产 Compose 的 API、Web 没有健康检查 | `admin health [live|ready]` 子命令（distroless 无 shell）；API 只读根文件系统、去掉全部 capability；Web 等 API 健康后启动 |
| 镜像 tag 漂移、无依赖与漏洞扫描 | 镜像固定到补丁版本；依赖升级直接提交到 main（不用 Dependabot PR）；`security.yml` 每周跑 govulncheck 与 `pnpm audit --prod`。扫描发现并修复：Go 1.26.0 → 1.26.9（标准库 35 个可达漏洞）、quic-go、axios、vue、lodash 等生产依赖（审计归零）；剩 1 个需 Go 1.26.10 修复 |
| 前端大 chunk，告警阈值调到 4 MB | 阈值恢复 800 KB；分出 vue / element-plus / echarts；只注册用到的 Element Plus 组件（`check-el-components.mjs` 防遗漏），生产构建经 `element-plus-entry.ts` 按组件子路径导入（element-plus 块 953 → 613 KB）；菜单搜索与拼音库改为首次打开时加载 |
| 依赖升级 | Element Plus 2.11 → 2.14：表格插槽的 row 改为通用类型，传给强类型函数处加断言（生成模板同步）；树选择的 props 类型漏了 value，集中在 `treeSelectProps()`；vue-tsc 3.3 会把模板属性里的 "//" 误判、连带后续 v-model 报错，相关占位符改为脚本常量。开发依赖在原大版本内升级（Vite 7.3、ESLint 9.39、prettier 3.9 等），`pnpm audit` 生产与开发依赖均为 0 |
| 前端单元测试 | Vitest（`pnpm test`，并入 `make web-check` 与 CI）：动态路由转换、登录跳转防开放重定向、错误处理、按钮权限、字典缓存、分页表格、字段错误、树工具、格式化、上传前检查。另有 `make codegen-web-check`：把各种表结构生成的前端代码格式化后跑 eslint 与 vue-tsc（CI 的 e2e 任务执行） |
| 未做 | 镜像签名与 SBOM、备份任务（README 生产清单已提示）、Go 1.26.10 发布后升级 |

**E2E 补充**

- **共享会话：** 全局初始化时只登录一次管理员并保存会话，所有用例复用；登录、退出类用例使用临时用户。原因是整套用例的登录次数会超过默认限流（每账号 5 分钟 10 次）。
- **API 上下文：** 用于以其他账号登录的 API 上下文必须显式清空 `storageState`。测试运行器会把配置中的管理员会话带进 `request.newContext()`，服务端按防会话固定规则会在新登录成功时结束这个旧会话。服务端的行为是正确的，问题出在测试辅助代码。
- **页面冒烟测试：** 逐个访问全部菜单页，出现 `pageerror` 或「Failed to resolve component」时失败。这个测试能发现 Element Plus 组件未注册导致的空白区域，此前 `el-input-tag` 就出过这个问题。

**交付前评审（安全 + 正确性）及处理**

| 评审项 | 处理 |
|---|---|
| 定时任务停机无上限：处理器的 context 从不取消，部署时可能等待最长 24 小时 | 新增 `ShutdownGrace`（默认 20 秒）：停机时先等待，超过宽限期后取消所有执行中的任务，记为「服务停止，执行被中断」 |
| 任务锁 TTL 等于最坏耗时：实例崩溃后任务被长时间跳过；超时后被放弃的处理器可能与下一次执行重叠 | 锁 TTL 改为 60 秒，执行期间每 TTL/3 续期（Lua 比较后续期）；锁一直保持到所有处理器 goroutine 真正返回为止 |
| 上传的大请求体限制和长超时在认证之前生效，匿名请求可以把连接占住 30 分钟 | 全局中间件不再处理这类路由；限制和超时由模块路由器在访问校验之后施加。公开路由禁止声明 `MaxBody`（启动校验）；签名下载在验签之后由处理器自己延长截止时间 |
| 任何人都可以通过连续输错密码，在所有地方锁住某个账号 | 硬锁定改为按「账号 + IP」计数；账号级别的失败计数只用来触发验证码。管理员解锁时清除该账号所有 IP 的锁 |
| 匿名请求可以写入操作日志、刷爆数据表；验证码接口没有限流 | 操作日志只记录已认证用户的请求（登录尝试已在登录日志中）；验证码每 IP 每分钟最多 30 次 |
| 操作日志可能以明文保存参数值中的密钥 | 脱敏正则扩展（pwd、apiKey、privateKey、authorization、cookie、otp 等）；当同一对象的 `key`、`name` 或 `code` 字段表明值是敏感的（包括中文「密码 / 密钥」等）时，`value` 字段也会脱敏。参数表单上醒目提示不要保存密钥 |
| 处理器 panic 的请求没有操作日志 | 中间件中先 recover，记录为 500，再重新 panic 交给 Recover 中间件 |
| 会话下线只保护超级管理员 | 通过 `auth.WithTargetGuard` 注入 IAM 的防越权规则：不能让权限比自己大的管理员下线。会话 ID 参数必须是 64 位十六进制 |
| 字典缓存和参数缓存的「回填覆盖失效」竞态 | 字典：失效时递增版本号，回填用 Lua 校验版本号未变才写入；参数：进程内缓存加代数，清空后旧的回填会被丢弃。字典不存在时 `HasDictValue` 返回 false，而不是报错 |
| 上传：写库成功后回读失败会误删对象；删除：在事务提交前就删了对象 | 上传的 INSERT 使用脱离请求的 context，写库成功后不再删除对象；删除先提交软删除，再尽力删除对象和记录，失败的由新的「清理已删除文件」任务（迁移 000011，每小时一次）补做 |
| S3 每次上传都预先分配 8 MiB | 按实际读取的数据量增长缓冲区 |
| 生成签名链接没有审计 | 签名链接接口标记为 `Sensitive`，计入操作日志 |
| 进程停顿后可能补发过期的触发；`Next()` 可能返回零值 | 超过去重窗口的旧触发直接跳过；零值时取消该任务的调度并记录日志 |
| 手动执行时，请求被取消会留下 running 记录 | 改用脱离请求的 context 写执行记录 |
| `@every 1s` 可以刷爆执行记录；任务参数可以把审计日志只保留 1 天 | 接口要求 `@every` 不小于 1 分钟（`Tuning.MinEvery`）；日志保留天数下限为 7 天（设置、任务参数、代码三处都检查） |
| 代码生成时，字段注释原样插入生成的代码 | 生成标签时去掉引号、反引号、反斜杠、`<>{}$`、注释符等 |
| 停机时没有等待 reconciler | 停机时等待其退出后再关闭数据库 |

**AI 接口（开发工具 → AI 接口）**

给 AI 编程助手的 HTTP 接口，见 [ai-agent-api.md](ai-agent-api.md)。要点：密钥只存 SHA-256（迁移 000014 `sys_agent_keys`），以创建人身份调用，每次请求重新加载权限快照；路由在会话守卫看来是公开的，由 devtools 的 `agentAuth` 鉴权（`TestEveryRouteIsProtected` 断言匿名访问返回 401）；CSRF 对「带 Bearer、无会话 Cookie」的请求放行；建表只接受结构化描述（`codegen.TableSpec`），生成迁移文件后用独立连接执行（迁移执行器关闭时会关掉传入的连接池）。

**剩余风险与未验证项**

- 文件模块的 S3 驱动只在内存模拟的 S3 上测试过；连接真实 MinIO 的用例（`TestS3AgainstRealServer`）需要设置 `ADMIN_TEST_S3_ENDPOINT` 才会运行。
- 服务器监控只在 macOS 上实测过；Linux 构建可以通过，伪文件系统按类型过滤。
- 除登录和改密以外的接口仍没有通用限流（同 P0）。
- 代码生成接口只在 `app.env=dev` 下可用；CLI 直接写本地文件，应只在开发机上使用。
- AI 接口的密钥等同于创建人在开发环境的代码生成权限，泄露后可以在开发库建表、往仓库写文件；应设有效期，不用时撤销。
- 定时任务按服务器本地时区解析。在有夏令时的时区里，切换当天落在跳变区间的任务会被跳过或执行两次（与系统 cron 相同）；这类服务器建议使用 UTC 或无夏令时的时区。
- 每次触发「至多执行一次」：抢到触发键之后，如果实例崩溃或 Redis 短暂故障，这一次触发会丢失，不会补跑。
- 访问日志里记录的是请求路径，签名下载的令牌会出现在日志中（令牌默认 10 分钟过期）。
- 在线会话列表对拥有 `auth:session:view` 的用户展示所有会话（不按数据范围过滤）；下线操作受防越权规则约束。
- 生产环境不强制 `file.s3.use_ssl`，方便在内网中使用 MinIO；跨网络访问对象存储时应开启 TLS。

