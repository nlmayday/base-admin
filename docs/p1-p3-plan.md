# P1–P3 implementation plan

> **状态：已完成。** 实施中的偏差与补充记录在 [rebuild-plan-v2.md §18](rebuild-plan-v2.md#18-p1p3-实施记录偏差与补充)。

This plan covers the work after P0: logs and auditing, system configuration, files, jobs and monitoring, developer tools and delivery. It extends [rebuild-plan-v2.md](rebuild-plan-v2.md) and follows the same rules (sections 4–7, 16, 17). Where they conflict, rebuild-plan-v2.md wins.

## 1. Modules and ownership

Each module is a compile-time module (`module.Module`) in its own package. Work on one module stays inside the paths it owns. Shared code (`server/internal/{app,core}`, `modules/{auth,iam}`, `platform/*`, `web/src/{router,layout,store,utils/http}`, `Makefile`, `README.md`, `docs/rebuild-plan-v2.md`) is changed only during integration.

| Module | Backend package | Frontend | Owns |
|---|---|---|---|
| auth (extended) | `modules/auth` | `views/security/*`, login page | online sessions, security policy, captcha, lockout, password rules |
| audit | `modules/audit` | `views/audit/*`, `api/audit/*` | login log, operation log, log stats, retention job |
| system | `modules/system` | `views/sysconfig/*`, `api/system/*`, `composables/useDict.ts` | data dictionary, config params with history, basic/site settings |
| file | `modules/file`, `platform/storage` | `views/file/*`, `api/file/*` | storage drivers, upload/download, media library, signed URLs |
| ops | `modules/ops` | `views/ops/*`, `api/ops/*` | scheduled jobs, run records, server/app monitor |
| devtools | `modules/devtools`, `internal/devtools/*`, `cmd/gen` | `views/devtools/*`, `api/devtools/*` | OpenAPI generation, API doc UI, code generator (CLI + preview) |
| ~~example~~ | ~~`modules/example`~~ | ~~`views/example/*`, `api/example/*`~~ | 已取消（不属于基础后台） |

## 2. Conventions every module follows

### Backend
- Each package exposes `New(deps Deps) *Module`, where `Deps` is a struct, and implements `module.Module`. Modules that contribute jobs also implement `jobs.Provider` (`internal/core/jobs`).
- Every route goes through `*module.Router` with an access level, and is annotated with `.Doc(module.Doc{Summary, Query, Body, Response, Sensitive})`. The summary is a short Chinese operation name; the operation log and API docs show it. Mark exports `Sensitive: true`. Upload routes call `.MaxBody(n)`.
- Patterns are the same as `modules/iam`:
  - use `handle(...)` adapters, `httpx.ParsePage`, `httpx.Page[T]`, `httpx.BindJSON`;
  - check versions with `updateVersioned`-style SQL (`version = version + 1 WHERE id AND version`), returning 409 on conflict;
  - use `database.RunTx` for multi-statement writes;
  - IDs in JSON are `ids.ID` strings, times are `time.Time` (RFC 3339);
  - user-facing messages are Chinese.
- Changes that affect authorization go through `auth.Core.Versions.Mutate`. Most new modules have none.
- Raw SQL with bound parameters is fine. Column and table names in SQL strings must be constants, never user input. Escape `LIKE` patterns.
- Files: functions under ~50 lines, files under 400–500 lines, no `console.log`, no secrets in code.
- Tests:
  - unit tests;
  - `//go:build integration` tests that use `testinfra/migrated.Database(t)` and `testinfra.RedisDB(t, <index>)`, mounting the module on a Gin engine with a test guard;
  - happy paths, validation, permission levels, and the module's specific risks.

### Numbering

| Module | Migration | Error codes | Redis test DB |
|---|---|---|---|
| audit | `000005_audit` | 40xxx | 4 |
| system | `000006_system` | 50xxx | 5 |
| auth security | `000007_auth_security` | 2xxxx (extend) | 2 (existing) |
| file | `000008_file` | 60xxx | 6 |
| ops | `000009_ops` | 70xxx | 7 |
| devtools | — | 80xxx | 8 |
| ~~example~~ | 已取消；000010、000011 由内置任务迁移使用 | ~~90xxx~~ | ~~9~~ |

### Permission codes and menus
- Menu codes are `<module>.<name>`; each module declares its own top-level directory. Sort values place the directories in this order:
  - 工作台 (static)
  - 系统管理 `iam.system` 90
  - 系统配置 `system.root` 91
  - 安全中心 `auth.security` 92
  - 日志审计 `audit.root` 93
  - 资源管理 `file.root` 94
  - 运维管理 `ops.root` 95
  - 开发工具 `devtools.root` 96
  - ~~示例业务 `example.root` 100~~（已取消）
- Codes:
  - **audit:** `audit:login-log:view|export`, `audit:operation-log:view|export`.
  - **system:** `system:dict:view|create|update|delete`, `system:config:view|create|update|delete`, `system:settings:view|update`.
  - **auth:** `auth:session:view|revoke`, `auth:policy:view|update`.
  - **file:** `file:file:view|upload|update|delete`, `file:storage:view`.
  - **ops:** `ops:job:view|create|update|delete|execute`, `ops:run:view`, `ops:monitor:view`.
  - **devtools:** `devtools:apidoc:view`, `devtools:codegen:view`. The API doc replaces the read-only `iam.api` page during integration.
  - ~~**example:** `example:customer:view|create|update|delete|export`~~ (cancelled).

### Frontend
- Vue 3 + TS + Element Plus, following the existing pages (`views/system/user`, `views/system/role`):
  - `usePagedTable`, `confirmAction`, `v-perm`, `hasPerm`, `ReStatusTag`, `formatDateTime`;
  - form dialogs with `:validate-on-rule-change="false"`;
  - field errors from 422 (`useFieldErrors`);
  - send `version` back on update;
  - IDs stay strings.
- View files live at the component path registered in the menu, e.g. `src/views/audit/login-log/index.vue` for `component: "audit/login-log/index"`. API wrappers go in `src/api/<module>/*.ts` using `http` from `@/utils/http`.
- Visual quality:
  - **Layout:** consistent spacing, card layout, and a filter card above the table card.
  - **Content:** clear empty states, loading states, and tags for status and type.
  - **Responsiveness:** tables don't overflow at 1366 px, and long text uses `show-overflow-tooltip`.
  - **Feedback:** confirmations for destructive actions, and success toasts.
  - **Charts:** use `echarts`, which is already a dependency.
- Run checks only on your own paths so that concurrent work does not interfere:
  - `pnpm exec eslint <paths> --fix`
  - `pnpm exec prettier --write <paths>`
  - `pnpm exec vue-tsc --noEmit --skipLibCheck` (ignore errors in files you do not own)

## 3. Module specifications

### audit
- **Tables:**
  - `sys_login_logs(id, kind login|logout, username, user_id NULL, success, reason, ip, user_agent, request_id, created_at)`.
  - `sys_operation_logs(id, user_id NULL, module, method, route, path, summary, status, code, success, duration_ms, ip, user_agent, request_id, request_body (redacted, ≤4 KB), error_message, created_at)`.
  - Indexes on `created_at DESC`, user and module.
- **Writer:**
  - Implements `auth.LoginObserver`.
  - Exposes `Middleware(reg *module.Registry) gin.HandlerFunc` for the API group.
  - Writes asynchronously: a bounded queue with batch inserts and `Run(ctx)` / flush on shutdown. Login latency must not depend on it. On overflow it waits briefly, then drops, logs and counts the drop. It never blocks indefinitely.
- **Middleware:**
  - **What it records:** every non-GET request plus GET routes with `Doc.Sensitive`, except `POST /auth/login`, which is covered by the login log. The actor comes from `actor.From` after `c.Next()` (the guard sets it); 401/403 attempts are recorded too.
  - **Body capture:** JSON request bodies up to 4 KB, restored for the handler. Values whose keys match `password|token|secret|captcha` (case-insensitive) are replaced by `***`. Multipart bodies are not captured.
- **API:**
  - `GET /audit/login-logs` with filters: username, success, ip, time range, page.
  - `GET /audit/login-logs/export` (CSV).
  - `GET /audit/operation-logs` with filters: user keyword, module, method, success, path keyword, time range.
  - `GET /audit/operation-logs/:id`.
  - `GET /audit/operation-logs/export`.
  - `GET /audit/stats`, accessible with any audit view code: today's login successes and failures, today's operations, and 7-day trends.
  - Logs are read-only: no delete API.
- **Job:** `audit.cleanup` deletes logs older than `params.days` (default 180).
- **UI:**
  - **Login log:** filters, a success/failure tag and a reason in Chinese.
  - **Operation log:** a detail drawer with pretty JSON, the method tag, a duration colour and an error message.
  - **Both:** date-range pickers with shortcuts.

### system
- **Tables:**
  - `sys_dict_types(code unique, name, status, remark, version, …)`.
  - `sys_dict_items(type_id FK cascade, label, value, sort, status, tag_type, remark, version)`, unique on `(type_id, value)`.
  - `sys_configs(key unique, value, value_type string|number|boolean|json, group_name, name, description, editable, builtin, version, …)`.
  - `sys_config_changes(config_id, key, old_value, new_value, changed_by, changed_at)`.
- **API:**
  - CRUD for dictionary types and items.
  - `GET /dicts/:code/items` (Authenticated; enabled items only; cached in Redis and invalidated on change).
  - Config CRUD, where built-in and non-editable entries are protected, plus `GET /configs/:id/changes`.
  - Typed settings: `GET/PUT /settings` (system:settings:*) and `GET /settings/public` (Public: site name and logo only).
- **Seeded built-in settings:**
  - `site.name` ("Base Admin");
  - `site.logo_url`;
  - `upload.allowed_extensions` ("jpg,jpeg,png,gif,webp,pdf,docx,xlsx,pptx,zip,txt,csv,md");
  - `upload.max_size_mb` (20);
  - `audit.retention_days` (180).
- **Go API for other modules:** `(*Module).UploadPolicy(ctx) (exts []string, maxBytes int64, err error)` and `GetString/GetInt(ctx, key)`, cached briefly.
- **UI:**
  - Dictionary page: types on the left, items on the right, and a tag colour preview.
  - Params page: table plus a change-history drawer.
  - Settings page: grouped form cards.
  - `useDict(code)` composable for selects and tags.

### auth (security centre)
- **Online sessions:** the store keeps a `sess:active` sorted set. `GET /sessions` (auth:session:view) lists sessions with user, IP, UA, created and last-seen times, and marks the caller's own session. `DELETE /sessions/:hash` and `POST /users/:id/sessions/revoke` (auth:session:revoke) end them.
- **Security policy:** a single-row `sys_security_policy` table, versioned:
  - password minimum length and character classes;
  - captcha after N failures, or always;
  - lockout after N failures for M minutes;
  - idle and absolute session timeouts in display only (config-owned).
- **Enforcement:** user create, reset and change apply the password rules; login applies captcha and lockout.
- **Captcha:** `GET /auth/captcha` returns `{id, image}` as a base64 PNG stored in Redis with a TTL. The frontend shows it on the login page when required.

### file
- **Config:**
  - `file.driver` local|s3, `file.local_dir`;
  - `file.s3.{endpoint,region,bucket,use_path_style,use_ssl}`, with the access key and secret only from the environment;
  - `file.signed_url_ttl`, `file.max_upload_bytes` (hard ceiling);
  - `file.signing_key` (env; a random value in dev).
- **Storage interface** in `platform/storage`, with local (path-safe, server-generated keys `yyyy/mm/dd/<uuid>.<ext>`) and S3-compatible (minio-go, which covers MinIO, AWS, OSS and COS) drivers.
- **Tables:**
  - `sys_file_categories(name unique, sort)`.
  - `sys_files(category_id NULL, original_name, object_key, storage, size, mime_type, ext, sha256, uploaded_by, created_at, deleted_at)`.
- **Validation:** an extension allow-list and size limit from `UploadPolicy`, capped by config. The sniffed MIME type must match the extension family. SVG and HTML are refused by default. Downloads send `nosniff`; only images and PDF are served `inline`, everything else as an attachment.
- **API:**
  - Upload, list (keyword, category, type group), get, rename/move, delete.
  - Authenticated download.
  - `GET /files/:id/signed-url` returns `{url, expiresAt}`; `GET /files/signed/:token` is Public, HMAC-checked and expiring.
  - Category CRUD.
  - `GET /files/storage` returns driver info and usage.
- **UI:**
  - **Media library:** a category sidebar, grid and list views, thumbnails, and a drag-and-drop multi-upload with progress.
  - **Preview:** a dialog for images and PDF.
  - **Actions:** copy a signed link, rename, move.
  - **Storage page:** driver info and usage.
  - Uploads use the shared axios client so cookies and XSRF are sent.

### ops
- **Tables:**
  - `sys_jobs(name, handler_key, cron_expr, params jsonb, enabled, timeout_seconds, max_retries, remark, version, …, last_run_at, last_status)`.
  - `sys_job_runs(job_id, handler_key, trigger cron|manual, status running|success|failed|timeout|skipped, attempt, started_at, finished_at, duration_ms, error, instance)`.
- **Scheduler:** robfig/cron v3 runs only handlers registered through `jobs.Provider`.
  - **Single execution across instances:** each fire takes a Redis lock (`SET NX EX`), so it runs once.
  - **Timeouts and retries:** the job's timeout comes through the context, with retries and backoff.
  - **Reloading:** changes propagate through Redis pub/sub, plus a periodic resync.
  - **Manual runs:** run now is asynchronous and returns the run id.
  - **Built-in job:** `ops.cleanup-runs`.
- **Monitor:**
  - `GET /monitor/server` (gopsutil): CPU, load, memory, swap, disks, host and uptime.
  - `GET /monitor/app`: Go runtime and GC, goroutines, DB pool stats, Redis info, build and uptime.
- **UI:**
  - **Jobs:** cron presets and a readable next-run preview, a handler select with descriptions, a params JSON editor, an enable switch and run now.
  - **Runs:** filters plus an error detail view.
  - **Monitor:** echarts gauges, auto-refresh and a short CPU/memory history chart.

### devtools
- **OpenAPI generator** from `module.Registry`:
  - reflects `Doc.Query/Body/Response` into JSON Schema (`ids.ID` → string, `time.Time` → date-time, `httpx.Page[T]` handled), wrapped in the response envelope;
  - adds cookie auth plus the XSRF header as security, tags per module, and an `x-permissions` extension.
- **API:**
  - `GET /devtools/openapi.json` and `GET /devtools/routes` for the UI.
  - `GET /devtools/codegen/tables` and `POST /devtools/codegen/preview`, which return generated files without writing anything. Only `app.env=dev` allows these; otherwise they return 403.
- **CLI:** `go run ./cmd/gen -table <t> -module <m> -resource <r> -name <中文名> [-force] [-dry-run]` reads the PostgreSQL schema and generates:
  - the backend module (module.go, resource handlers/service, permissions, menus, integration test skeleton);
  - the frontend API and views (list, filter, form dialog).
  - It refuses to overwrite existing files without `-force`.
- **UI:**
  - API doc: modules on the left with search; on the right the route detail, access tags, a parameter table, schema trees and example JSON; download of openapi.json.
  - Codegen: pick a table, then preview the files with syntax highlighting, and copy the CLI command.

### example (P3) — 已取消

The example business module (`biz_customers`, 客户管理) was built and then removed: business modules are outside the scope of the base admin system. The code generator and `docs/module-guide.md` cover how to add one.

### Delivery (P3)
- The delivery work:
  - `docs/module-guide.md`;
  - `scripts/new-project.sh`, which renames the Go module path and project name;
  - `server/Dockerfile` (multi-stage, non-root) and `web/Dockerfile` (nginx serving dist and proxying /api);
  - `deploy/nginx/nginx.conf` and `deploy/docker/compose.prod.yaml` (postgres, redis, a migrate one-shot, api, web).
