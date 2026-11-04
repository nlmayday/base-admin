# base-admin

[简体中文](README.md) | English

A reusable admin system template: a Go + Gin backend plus a Vue 3 + Element Plus frontend built on pure-admin-thin. The user interface is in Chinese; the Chinese menu names are given in parentheses below. The architecture baseline is [docs/rebuild-plan-v2.md](docs/rebuild-plan-v2.md), the P0 HTTP contract is [docs/api-p0.md](docs/api-p0.md), and the P1–P3 feature plan is [docs/p1-p3-plan.md](docs/p1-p3-plan.md). Some documents under `docs/` are in Chinese.

## Features

| Menu | What it does |
|---|---|
| Dashboard (工作台) | Greeting, permission-aware statistic cards, quick links, system status, and 7-day login and operation trends |
| System (系统管理) | Users (with CSV import: template, row-by-row errors, all or nothing), roles (menu, button and API permissions, data scope), departments, positions, menus (icons picked from a built-in offline icon set) |
| Settings (系统配置) | Data dictionaries (with tag styles), system parameters (typed, change history), basic settings (site name and logo, upload rules, log retention) |
| Messages (消息中心) | Notices with attachments: drafts, publish and revoke; personal messages that business modules send to one user (a click opens the business page); both show in the header bell (unread count) and in My messages (我的消息) |
| Security (安全中心) | Online sessions (force logout per session or per user), and a security policy covering password complexity, captcha (off, after N failures, always), account lockout, an IP allowlist and login hours (with exempt roles); business modules can register their own login checks |
| Audit (日志审计) | Login logs and operation logs (route summary, duration, result, request ID, sensitive-field masking), with statistics and CSV export |
| Files (资源管理) | Image and attachment upload components for business forms (any signed-in user, links that need a login but no file permission); media library (categories, grid and table views, preview, signed links, drag-and-drop upload with content sniffing) and storage overview; local disk or S3/MinIO |
| Operations (运维管理) | Scheduled jobs (cron with preview, retries, timeouts, single run per fire across instances, manual runs) and run history; server and application monitoring |
| Dev tools (开发工具) | API docs generated from the route registry (and OpenAPI 3 JSON), plus a code generator that turns a table into a working module (dev only). For each field you choose its Chinese name, whether it shows in the list and the form, and how it is searched; the module is then written into the project and registered automatically. **AI API (AI 接口)**: keys for AI coding agents, which create tables and generate modules over HTTP following the same conventions ([docs/ai-agent-api.md](docs/ai-agent-api.md)) |

Design points:
- **Modular monolith:** every feature is a `module.Module` with its own permission codes, menus, routes and jobs. Startup validation rejects unprotected or undeclared routes.
- **Opaque sessions in Redis:** revocation through a PostgreSQL-backed session generation; CSRF protection through an Origin check plus a session-bound double-submit token.
- **Authorization:** permission codes with dependencies, data scope on every query, and anti-escalation rules for roles and departments.

## Requirements

- Go: the version is declared in `server/go.mod`, and `GOTOOLCHAIN=auto` downloads it.
- Docker with Compose v2.
- Node ≥ 20.19 and pnpm ≥ 9, for the frontend.

## Quick start

```bash
make deps-up          # PostgreSQL 127.0.0.1:15432, Redis 127.0.0.1:16380
make migrate-up       # apply schema migrations (tables and default data)
make run              # API :8080, metrics 127.0.0.1:9090
make web-install
make web-dev          # UI http://localhost:8848 (proxies /api to API_URL)
```

Log in as `admin` with the password in `ADMIN_BOOTSTRAP_ADMIN_PASSWORD` (Makefile development default: `Admin@dev-2026`). You must change it at first login.

If port 8080 is taken, run the API elsewhere and point the frontend at it:

```bash
ADMIN_HTTP_ADDR=:18080 make run
API_URL=http://127.0.0.1:18080 make web-dev
```

MySQL 8.4 can replace PostgreSQL: `make deps-up-mysql`, `make migrate-up-mysql`, `make run-mysql`. The conventions and limits are in [docs/mysql.md](docs/mysql.md) (in Chinese); the code generator and the agent table API work on both databases.

`make help` lists every target. `make ci` runs build, lint, the backend unit and integration tests, the frontend lint and type-check, and the frontend build.

### Initial data

Nothing has to be imported by hand, but **the migrations must run first**: `make run` exits with a hint when the schema is not at the latest version.

- `make migrate-up` creates the tables and the default data: data dictionaries, the security policy and the built-in scheduled jobs (log, file and personal-message cleanup).
- Every start syncs the permission codes, menus and system parameters of all modules, so new menus and permissions appear after a restart.
- When no user exists yet, the server creates the super admin role and the `admin` account with the password in `ADMIN_BOOTSTRAP_ADMIN_PASSWORD`; without that variable it refuses to start.

No demo business data is created.

## Configuration

Precedence, lowest to highest: built-in defaults, an optional YAML file (`-config` or `ADMIN_CONFIG`), then `ADMIN_*` environment variables. [server/configs/config.example.yaml](server/configs/config.example.yaml) lists every key; unknown keys and bare-number durations are rejected.

Secrets are read only from the environment, and the server refuses to start if a config file contains them:

| Variable | Purpose |
|---|---|
| `ADMIN_DATABASE_URL` | Database URL (required): `postgres://…` or `mysql://…`; the scheme picks the database |
| `ADMIN_REDIS_PASSWORD` | Redis password |
| `ADMIN_BOOTSTRAP_ADMIN_PASSWORD` | Creates the first super admin when no user exists (12–72 characters) |
| `ADMIN_FILE_SIGNING_KEY` | HMAC key for signed file links, at least 32 bytes (required in prod) |
| `ADMIN_FILE_S3_ACCESS_KEY` / `ADMIN_FILE_S3_SECRET_KEY` | S3/MinIO credentials when `file.driver=s3`; omit both to use the AWS/MinIO environment or an instance role |

Runtime settings live in the database and are edited in the UI: Basic settings (基础设置) for the site name, logo, upload rules and log retention, and Security → Security policy (安全中心 → 安全策略) for password, captcha and lockout rules.

## Deployment

`deploy/docker/compose.prod.yaml` runs the whole stack:
- PostgreSQL and Redis (password-protected, `noeviction`, AOF);
- a one-shot `migrate` service;
- the API, as a distroless non-root image with a read-only root file system and no capabilities; its healthcheck runs `admin health ready` (PostgreSQL and Redis reachable);
- nginx, serving the SPA (hashed bundles under `/static/` cached for a year, `index.html` never) and proxying `/api`; healthcheck `/healthz`.

Image tags are pinned to patch versions. Dependency upgrades are committed directly to main, without pull requests; `security.yml` scans for vulnerabilities weekly and flags what needs upgrading.

```bash
cp deploy/docker/prod.env.example deploy/docker/prod.env   # fill in every value
docker compose -f deploy/docker/compose.prod.yaml --env-file deploy/docker/prod.env up -d --build
```

Uploaded files (local driver) are kept in the `files` volume. For several API replicas, use `file.driver=s3`: scheduled jobs already run once per fire across instances.

Production checklist:
- Set `ADMIN_APP_ENV=prod`.
- Use cookie name `__Host-sid` with `auth.cookie_secure=true` (enforced in prod).
- Set `auth.allowed_origins` to the real frontend origin.
- Set `http.trusted_proxies` to your load balancer or ingress network.
- Set `http.drain_delay`.
- Give each deployment its own Redis logical database with `noeviction` or a `volatile-*` policy.
- Set `ADMIN_FILE_SIGNING_KEY`.
- Keep the metrics port (`metrics.addr`) off the public network.
- Back up PostgreSQL (and the `files` volume, or the S3 bucket) on a schedule and rehearse a restore; take a snapshot before `migrate up`. The stack does not include a backup job.

## Commands

```text
admin serve                      run the API server (default)
admin migrate up                 apply pending migrations (scaffold track first, then app)
admin migrate version            print the version of each track (exit 1 if dirty)
admin migrate <track> down <n>   roll back n migrations of a track
admin migrate <track> force <v>  recover from a dirty state (v = existing version, or -1 for none)
gen -table … -module …           generate a module from a table (go run ./cmd/gen -h)
```

Migrations come in two tracks, each numbered independently with its own version table: `server/migrations` is the scaffold's (`schema_migrations`), `server/migrations/app` is the application's (`app_schema_migrations`). Migrations added upstream and downstream never collide when you merge. Tracks are named `base` and `app`; `up` and `version` can also target one, e.g. `admin migrate app up`.

`serve` refuses to start unless the schema is at the latest version, so run `migrate up` as a separate deployment step. At startup the server validates every module, then syncs permission codes, menus and seed data. The first SIGTERM or Ctrl-C starts a graceful stop: in-flight requests drain, running jobs finish, and the audit log flushes. A second signal exits immediately.

## Starting a new project from this template

```bash
scripts/new-project.sh github.com/acme/crm/server "Acme CRM" acme-crm
```

The script rewrites the Go module path everywhere: imports, the code generator's templates, the Dockerfile and lint config. It also changes the product name and the compose project names, then checks that the backend still builds.

## Adding a business module

See [docs/module-guide.md](docs/module-guide.md). In short:
1. Run `make new-migration NAME=create_<table>` for a table template in `server/migrations/app` with the standard columns, fill in the business columns and comments ([docs/table-guide.md](docs/table-guide.md)), then run `make migrate-up`.
2. In Dev tools → Code generator (开发工具 → 代码生成):
   - pick the table;
   - set each field's Chinese name, whether it shows in the list and the form, where its select options come from (the column comment or a data dictionary), and how it is searched (contains, exact or range);
   - click **Generate into project (生成到项目)**.

   It writes the backend and frontend files and registers the module automatically. The CLI `go run ./cmd/gen … -fields '<JSON>'` does the same.
3. Restart the backend, then grant the new permission codes to a role.

Steps 1–2 can also be done over HTTP. Create a key in Dev tools → AI API (开发工具 → AI 接口), then either
- give an AI coding agent the copied instructions: it reads the usage guide from the API, creates the table (migration file included) and generates the module; or
- call it yourself with curl. Two requests create the table and its CRUD module:

  ```bash
  curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    $BASE/tables -d '{"name":"biz_items","label":"商品","columns":[{"name":"title","type":"string","label":"名称","required":true}]}'
  curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    $BASE/generate -d '{"table":"biz_items","module":"shop","resource":"item","name":"商品","moduleTitle":"商城管理"}'
  ```

  `BASE` is `http://localhost:8848/api/v1/devtools/agent` (development only). The full example and field options are in [docs/ai-agent-api.md](docs/ai-agent-api.md#用-curl-直接调用) (Chinese).

## Tests

- `make test`: unit tests; needs no dependencies.
- `make test-integration`: adds the tests tagged `integration`, which run against the Compose services.
  - Each test creates and drops its own PostgreSQL database and leases a Redis logical database, which is flushed and returned when the test ends.
  - Only empty databases 1–15, or ones left by earlier test runs, are leased; database 0 and databases holding other data are never touched. Tests can therefore share Redis with your development server, and several test runs can go at once. Keep the development server on database 0 (the default) or on a database that already holds data.
  - If the environment variables are missing, these tests fail rather than skip.
- `make web-check`: frontend lint (check only), type-check and Vitest unit tests (`pnpm test`; `pnpm test:watch` while developing).
- `make codegen-web-check`: generates the frontend for several table shapes, then formats, lints and type-checks it.
- `make e2e`: Playwright against a running backend.
  - Set `E2E_ADMIN_USERNAME` and `E2E_ADMIN_PASSWORD` to an admin whose initial password has already been changed, or also set `E2E_BOOTSTRAP_PASSWORD` on a fresh database: the suite then changes the bootstrap password to `E2E_ADMIN_PASSWORD` first (CI does this).
  - The suite logs that admin in once and reuses the session; logins and logouts use temporary users.
- CI (`.github/workflows/ci.yml`): backend build, lint and integration tests; frontend checks (`pnpm lint` only checks, `pnpm lint:fix` fixes locally) and build, then `git diff --exit-code`; Playwright E2E on a fresh database. `security.yml` runs govulncheck and `pnpm audit --prod` weekly and on main.
- On commit, git hooks format and lint the staged frontend files and require a conventional commit message such as `feat: ...` or `fix: ...`.
