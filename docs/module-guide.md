# Adding a business module

Downstream projects add features as **modules**. A module is a Go package that plugs into the core through one interface, plus its pages in the frontend. Adding a module never requires changes to `core`, `platform`, `auth` or `iam`.

This guide shows two routes: generating a module from a table (recommended), and writing one by hand. An AI coding agent can do the generation for you through the AI agent API ([ai-agent-api.md](ai-agent-api.md)): it creates the table from a structured description and calls the same generator.

## 1. Generate a module from a table (recommended)

1. **Write the migration.** Run `make new-migration NAME=create_<table>`: it creates `server/migrations/app/NNNNNN_create_<table>.{up,down}.sql` from a table template with the standard columns (see [Migration tracks](#migration-tracks)). Edit the business columns and comments, then run `make migrate-up`. The conventions are in [table-guide.md](table-guide.md) (Chinese). To get the most out of the generator, the table should have:
   - `id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY`
   - `version integer NOT NULL DEFAULT 0` for optimistic locking
   - `created_at`, `updated_at`, `created_by`, `updated_by`
   - optional `deleted_at` (soft delete), `status smallint` (1 enabled, 2 disabled), and `sort`
   - optional `dept_id` and/or `created_by`, which turn on data-scope filtering by department or owner
   - column comments, which become the field labels; a comment such as `类型：1=通知,2=公告` also lists the column's options, which become a select
2. **Configure and generate** in 开发工具 → 代码生成 (development environment only):
   1. Pick the table.
   2. Fill in the module name, resource name and Chinese name.
   3. Adjust the **field configuration**, one row per column:

   | Setting | Meaning |
   |---|---|
   | 中文名 | Label used for the table header, the form and the search item; defaults to the column comment |
   | 列表显示 | Show the column in the list page |
   | 表单 | Let users set the column; locked for system columns and for NOT NULL columns without a default |
   | 控件 | The input of the column. **Relation:** a foreign key column (`customer_id bigint REFERENCES biz_customers (id)`) becomes a searchable select of the referenced rows; the list, the detail and the export show the chosen row's label column (`customerName`), which you can pick among the referenced table's text columns. The generated module adds `GET /<resources>/options/<field>?keyword=` and checks on save that the row exists (through `internal/core/lookup`). **Upload:** 图片上传 for text columns of at least 255 characters (stores the image URL; the list shows a thumbnail), 附件上传 for jsonb columns (stores `[{name, url, size}]`; the list shows the count). Both are picked by default from the column name or comment (`cover_url`, `avatar`, 封面, 图片; `attachments`, 附件), are not searchable, and the generated backend accepts only attachment URLs. **Options:** where an editable text or integer column's options come from: none (an input), the options in the column comment (written into the code, checked by the request binding), or a data dictionary (系统配置 → 数据字典; maintained at run time, checked against enabled items on save). The form and the search use a select, the list shows the label, and the column is searched only by 精确. Comment options are chosen by default |
   | 搜索方式 | 不搜索; 模糊 (contains, text only); 精确 (equals); 范围 (between two values: numbers, amounts, dates, times) |

   Each searched field gets its own control in the generated list page: an input, a 是/否 select, the status select, a department tree, a date picker, a min–max pair, or a date-range picker. Defaults need no change: up to two text columns (unique ones first) are 模糊 and the status is 精确. The list shows up to eight columns by default; nullable unbounded `text` columns (usually long notes) are hidden.

   The dictionary must exist before generating; the generated module receives the system module as its `Dict` dependency (`helpdesk.New(helpdesk.Deps{DB: d.db, Dict: systemModule})`). System dropdowns whose values have meaning in code (启用/停用, data scope, captcha mode) stay fixed and are not dictionaries.

   Then:
   - **生成预览** shows the files without writing anything.
   - **生成到项目** writes them, formats the frontend files, and registers the code:
     - a new module is added to `server/internal/app/modules.go`, above the `// codegen:modules` marker;
     - a resource added to an existing module is added to that module's `module.go`, above its `// codegen:permissions`, `// codegen:menus` and `// codegen:routes` markers.

     Existing files are kept unless 覆盖已有文件 is ticked. The Vite dev server reloads the page when new view files appear; the page then restores the form and shows the result.

   The same works from the CLI. The page shows the equivalent command, including the field configuration:
   ```bash
   cd server
   go run ./cmd/gen -table biz_orders -module sales -resource order -name 订单 -module-title 销售管理 \
     -fields '[{"column":"amount","search":"range"},{"column":"remark","list":false}]' -dry-run
   ```
   Flags:
   - `-fields`: the field configuration as JSON; only the columns that differ from the defaults are needed.
   - `-module-title`: the title of the new module's menu directory.
   - `-parent-menu`: attaches the page to an existing directory instead of creating one.
   - `-dsn`: the database URL; defaults to `$ADMIN_DATABASE_URL`.
   - `-force`: overwrites existing files.
   - `-no-format`: skips Prettier on the web files.
   - `-no-wire`: skips the registration in `modules.go` or `module.go`.

   The generator writes:
   - `server/internal/modules/<module>/module.go` and `common.go` (only for a new module), `<resource>.go` and `<resource>_integration_test.go`. The test covers create/read/update/delete, optimistic locking, duplicates on unique columns, validation and every search parameter.
   - `web/src/api/<module>/<resource>.ts`
   - `web/src/views/<module>/<resource>/index.vue` and `components/<Resource>FormDialog.vue`

   What the generator supports:
   - **Column types:** text, integer and numeric, boolean, date and time, uuid, json and inet. `*_id` columns become string IDs.
   - **Skipped:** array, bytea and interval columns, and columns whose names look like secrets (password, token, …). The generator prints a note for each.
   - **Other behaviour:**
     - A duplicate value on a column with a unique index is reported on that field ("订单号已存在"), not as a 409.
     - Rows are named by a unique text column in the delete confirmation.
     - The export shows department names instead of IDs.
3. **Restart and grant.** Restart the backend (`go run` does not rebuild on its own). At startup it validates the module, then syncs its permission codes and menus into the database. Grant the new codes to a role in 角色管理 → 分配权限. Super admins have them automatically, and the menu appears for users holding the view code.

   Modules written by hand, or generated before the markers existed, are registered by hand instead:
   - add `sales.New(sales.Deps{DB: d.db}),` to the module list in `modules.go`;
   - add the resource's `xxxPermissions`, `xxxMenus` and `m.xxxRoutes(r)` to its `module.go`.

   The page prints the exact lines when it cannot do it.

The generated code is a starting point. Review the validation rules, the list filters and the form layout before you ship it. Typical refinements after generation:
- **Dictionary-backed fields.** For a column such as `status` backed by the `order_status` dictionary:
  - Seed the dictionary in your migration (`sys_dict_types`, `sys_dict_items`, `ON CONFLICT DO NOTHING`).
  - Validate the value in the backend through a small interface in your `Deps`, e.g. `HasDictValue(ctx, code, value) (bool, error)`, which the system module implements.
  - Use `useDict("order_status")` in the frontend for the select, the tags and the filter.
- **The generated integration test.** Extend it with the module's own rules.

## 2. Write a module by hand

Use a generated module or `modules/iam` as the reference. A module implements `module.Module`:

```go
type Module interface {
    Name() string                      // also the permission-code prefix
    Permissions() []perm.Def           // <module>:<resource>:<action>; non-view codes require view by default
    Menus() []menu.Def                 // <module>.<name>; page menus must reference a view code
    Routes(r *module.Router)           // every route declares an access level
    Seed(ctx context.Context, tx *gorm.DB) error // idempotent initial data
}
```

Rules that startup validation enforces (the server refuses to start if one is broken):

- **Routes and permission codes:**
  - Every route goes through the `*module.Router` it receives, with `access.Public()`, `access.Authenticated()`, `access.Perm(code)` or `access.AnyPerm(codes...)`.
  - A route cannot reference a permission code nobody declared.
  - Routes registered directly on Gin under `/api` are rejected.
- **Menus:** page menus reference their resource's `view` code; directories reference none. Any `Sort` works: the scaffold's directories use 90–96, so a smaller value puts yours above them, and the scaffold's tests find menus by code, not by position.
- **Code format:** codes and menu codes start with the module name.

Conventions the reviewers expect:

- **Route docs:** annotate every route with `.Doc(module.Doc{Summary: "新增订单", Body: req{}, Response: view{}})`. The summary appears in the operation log and the API docs. Mark exports `Sensitive: true`.
- **Data scope:** apply it in every query (list, detail, update, delete, export, options) with `actor.Scope(ctx, query, datascope.Spec{DeptColumn: "o.dept_id", OwnerColumn: "o.created_by"})`. A row outside the scope returns 404.
- **Optimistic locking:** update with `version = version + 1 WHERE id = ? AND version = ?` and return 409 when no row matched.
- **Authorization changes:** anything that changes authorization (roles, grants, departments) goes through `auth.Core.Versions.Mutate`. Ordinary business data does not.
- **Jobs:** implement `jobs.Provider` (`Jobs() []jobs.Handler`) to contribute job handlers, each with a key, a name, a description and a default timeout. Administrators schedule them in 运维管理 → 定时任务. If a handler must run by default, insert its job row in your migration (see `000011_audit_cleanup_job`). The scheduler guarantees one run per fire across instances. Handlers must honour `ctx` cancellation.
- **Personal messages:** to notify one user (a follow-up reminder, an assignment), take a `message.Sender` (`internal/core/message`) in your `Deps` and pass `noticeModule` in `internal/app/modules.go`. `Send(ctx, tx, message.Message{UserID, Category: "crm.reminder", Title, Content, Link: "/crm/customer?id=42"})` writes in your transaction when `tx` is not nil. The message shows in the header bell and in 我的消息 → 个人消息; clicking it opens `Link` (a frontend route starting with `/`). Recipients that do not exist or are deleted are skipped. The job 清理个人消息 deletes read messages after 90 days.
- **Login rules:** IP allowlists and login hours are built into 安全中心 → 安全策略. For a rule of your own (a contract expired, a tenant suspended), implement `auth.LoginGuard` and register it with `core.AddLoginGuard(yourModule)` in `internal/app/modules.go`. It runs after the password check and before the session is created; return an `*httpx.Error` (403, a code in your module's range) to refuse with your own message. Refusals are logged with reason `denied`.
- **Large uploads and downloads:** chain `.MaxBody(n)` and `.Timeout(d)` on the route. Only that route gets the larger body limit and the longer request and connection deadlines.
- **Settings and dictionaries:** read runtime settings through the system module (`GetString`, `GetInt`) and validate dictionary values with `HasDictValue`. Take them as small interfaces in your `Deps` instead of importing the system package.
- **Integration tests:** use the `integration` build tag with `testinfra/migrated.Database(t)` and `testinfra.RedisDB(t)`, which leases a Redis logical database for the test. To simulate Redis data loss mid-test use `testinfra.FlushRedis(t, client)`, not `FLUSHDB`, which would drop the lease.

## 3. Frontend pages

- **Where views live:** a menu's `component` is a path under `web/src/views` without the extension, e.g. `sales/order/index` → `web/src/views/sales/order/index.vue`. There is no frontend route table to edit: routes come from `/me/menus`.
- **Button visibility:** gate buttons with `v-perm="'sales:order:create'"` or `hasPerm(...)`. The backend enforces the same codes.
- **Reuse the shared building blocks:** `usePagedTable`, `useFieldErrors`, `confirmAction`, `useDict(code)` (`items`, `labelOf`, `tagTypeOf`), `ReStatusTag`, `formatDateTime`, `formatBytes`, and the shared `http` client (it handles cookies, the XSRF header and the error codes).
- **Menu icons:** pick them in 菜单管理 (about 100 Remix icons, bundled for offline use). To offer more, add names to `web/scripts/gen-menu-icons.mjs`, run it, and format the output. Other Iconify names typed by hand load from the Iconify API.
- **Images and attachments in forms:** `<ReImageUpload v-model="form.coverUrl" />` (one image; store the URL in a `varchar(512)` column) and `<ReFileUpload v-model="form.attachments" :limit="5" />` (a list of `{name, url, size}`; store it in a `jsonb NOT NULL DEFAULT '[]'` column), from `@/components/ReUpload/`. They upload to `POST /files/attachments`, which any signed-in user may call under the upload rules of 基础设置. The returned URL contains a random key and opens for signed-in users only, without the media library permission. On the backend, validate what the client sends with `internal/core/attachment` (`attachment.ValidURL` for one URL, `attachment.Validate` for a list), so only attachment URLs are stored. The notice module is a complete example.
- **Element Plus components:** only the components the project uses are registered, in `web/src/plugins/elementPlus.ts`; `pnpm lint` (`scripts/check-el-components.mjs`) fails when a page or a codegen template uses an `<el-*>` tag that is not registered. Production builds resolve `import { ... } from "element-plus"` to `web/src/plugins/element-plus-entry.ts`, which re-exports from per-component paths (the package's main entry would bundle every component); a value missing there fails the build with "is not exported", so add it there too.
- **Keep IDs as strings,** send `version` back on update, and set `:validate-on-rule-change="false"` on forms whose rules change.

## 4. Removing a module

1. Delete its package and its line in `server/internal/app/modules.go`.
2. Delete its views and API files under `web/src`.
3. Add a migration that drops its tables. Never edit or delete migrations that have been applied.

On the next start, its menus are removed and its permission codes are marked deprecated. Existing grants stay in the database but no longer do anything.

## Migration tracks

Migrations come in two tracks, each numbered from 000001 with its own version table:

| Track | Directory | Version table | Who adds migrations |
|---|---|---|---|
| `base` | `server/migrations` | `schema_migrations` | the scaffold (upstream) |
| `app` | `server/migrations/app` | `app_schema_migrations` | your project |

`admin migrate up` (`make migrate-up`) runs `base`, then `app`, so business tables can reference `sys_*` tables. `serve` refuses to start unless both are current. `make new-migration`, the AI agent API and the generator's hints all use `app`; pass `TRACK=base` only when you change the scaffold itself. Because the numbers never mix, `git pull upstream main` can add scaffold migrations at any time and `migrate up` runs them, whatever version your `app` track has reached.

`down` and `force` change one track and must name it: `admin migrate app down 1`, `admin migrate base force 15`.

### Upgrading a project that kept business migrations in `server/migrations`

Before the tracks existed, business migrations shared the scaffold's numbering. To move them, on a branch that merges the scaffold:

1. Move your business migrations to `server/migrations/app` and renumber them from 000001, keeping their order. The scaffold's files in `server/migrations` must match upstream exactly.
2. On every database (development, staging, production), before running the new binary, record what has already run. `B` is the last scaffold migration the database actually ran (check `server/migrations` as it was before the merge), `A` the number of business migrations it ran:

   ```bash
   admin migrate base force B   # the scaffold migrations that really ran
   admin migrate app force A    # the business migrations, now numbered 1..A
   admin migrate up             # runs the scaffold migrations after B, then any newer business ones
   ```

   Back up first. If a scaffold migration after `B` conflicts with something your business migrations created, fix that before `migrate up`.

