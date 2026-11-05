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
   - `-key-mode`, `-read-only`, `-no-soft-delete`, `-no-create`, `-no-delete`, `-order-by`, `-children`: the options below.
   - `-spec specs.json`: generates a list of tables in one run. Each entry has the fields of the page's request (`table`, `module`, `resource`, `name`, `moduleTitle`, `parentMenu`, `menuTitle`, `menuSort`, `fields`, `keyMode`, `readOnly`, `softDelete`, `noCreate`, `noDelete`, `orderBy`, `children`); with `-force` the same file regenerates them.
   - `-module-title`: the title of the new module's menu directory.
   - `-menu-title`, `-menu-sort` (`menuTitle`, `menuSort` in a spec): the page's menu title (default 「<name>管理」, e.g. 「玩家查询」) and its order among its siblings (smaller first, default 1). Menus get their title and sort when first written to the database; later changes are made in 菜单管理, so set them before the first start.
   - `-parent-menu`: attaches the page to an existing directory instead of creating one.
   - `-dsn`: the database URL; defaults to `$ADMIN_DATABASE_URL`.
   - `-force`: overwrites existing files, except the extension files (below).
   - `-no-format`: skips Prettier on the web files and `gofmt -s` on the Go files. Both run by default; gofmt is the one of the toolchain `server/go.mod` selects (`go env GOROOT`), the formatter `make lint` checks with, so generated code passes `make lint` as it is (shared helpers in `common.go` are referenced so `unused` does not report the ones a resource does not call).
   - `-no-wire`: skips the registration in `modules.go` or `module.go`.

   The generator writes:
   - `server/internal/modules/<module>/module.go` (only for a new module), `common.go` (the helpers the generated resources share; rewritten on every generation into the module, so do not edit it), `<resource>.go` and `<resource>_integration_test.go`. The test covers create/read/update/delete, optimistic locking, duplicates on unique columns, validation and every search parameter.
   - `web/src/api/<module>/<resource>.ts`
   - `web/src/views/<module>/<resource>/index.vue` and `components/<Resource>FormDialog.vue`
   - The extension files `<resource>_ext.go` and `components/<Resource>Extension.vue`, written only when they do not exist and never overwritten, not even with `-force`, so hand-written code there survives regeneration:
     - `<resource>Restrict(ctx, q)`: a fixed condition on every query of the resource (list, detail, export, update, delete), e.g. `q.Where("t.is_test = ?", false)`. List counts run on the table without the joined tables (they join by key and add no rows), so use `t.<column>` here.
     - `<resource>BeforeUpdate(ctx, tx, old, req)` and `<resource>BeforeDelete(ctx, tx, old)`: run inside the transaction before the row is written or deleted, with the row as it was (`old`, the detail view) and the request (which may be adjusted); an error rolls back, e.g. 「活动开始后目标不能改」 from `old.StartAt`, 「当天的任务不能删」 with `httpx.Conflict(...)`. Both are optional: extension files from before them still compile; add the method (same signature) to use it.
     - `<resource>AfterSave(ctx, tx, row, created)` and `<resource>AfterDelete(ctx, tx, key)`: run inside the write's transaction after the row is written or deleted; an error rolls it back (`fieldError("price", "…")` reaches the form).
     - `<resource>AfterCommit(ctx, op, key, row)`: runs after the transaction is committed (`op` is `opCreate`, `opUpdate` or `opDelete`; `row` is nil after a delete), so other services see the new data, e.g. to tell a game server to reload its cache. An error is only logged («after-commit hook failed»); the save stands. It runs before the response, with a context that is not cancelled with the request; start a goroutine for slow calls. Extension files from before this hook have no `AfterCommit`; add the method (same signature) to use it.
     - Read-only resources have only `Restrict`; without delete (`noDelete`) there is no `BeforeDelete` or `AfterDelete` (delete an old `AfterDelete` from the extension file when turning delete off, or `make lint` reports it unused).
     - `<Resource>Extension.vue` renders in the list's toolbar (`place="toolbar"`) and in each row's 操作 column (`place="row"`, with `row`); emit `reload` to refresh the list, and set `hasRowActions = true` when it adds row buttons.

   **Tables owned by another service** (a game server's tables, created by its migrations) need no change for the generator; 高级选项 on the page (and the same JSON keys in the CLI) cover them:
   - **Primary key** (`keyMode`). Any single-column bigint, integer or text key. `auto`: the database assigns it (IDENTITY, AUTO_INCREMENT). `generated`: a bigint the module assigns on create with `NextID()`, from `Deps.NewID` (default `ids.Next`, a snowflake id); pass the other service's generator in `modules.go` when both write the table. `input`: typed in the create form and never changed, such as `cfg_key` or `level`; a duplicate is a field error. By default, database-assigned keys are `auto`, other bigint keys `generated`, the rest `input`.
   - **Read-only** (`readOnly`): only the list, the detail and the export (with the `:view` and `:export` codes) for ledgers, orders and logs. Fields of a read-only module can still have options (`fields[].dict`), for the list's labels, the search select and the export.
   - **Soft delete off** (`"softDelete": false`, `-no-soft-delete`): a nullable `deleted_at` normally hides deleted rows; when it means something else, such as the time an account was closed, turn soft delete off: `deleted_at` becomes a plain time column (listed, searchable), every row is listed, and delete removes the row.
   - **No create, no delete** (`"noCreate": true`, `"noDelete": true`, `-no-create`, `-no-delete`): leave out the route, the permission code, the button and the extension hooks, e.g. games are taken off sale (a status) and never deleted. Update stays.
   - **Order** (`"orderBy": "created_at DESC, id DESC"`, `-order-by`): the order of the list and the export, columns of the table with `ASC` or `DESC`; default `sort` (when the table has it), then the key descending. On a large ledger pick an order an index serves, e.g. `(created_at, id)`, rather than the default key scan.
   - **Declared relations** (`fields[].refTable`, `refKey`, `relation`): `{"column":"user_id","refTable":"lg_user","relation":"nickname"}` shows a column as the name of the referenced row without a foreign key; `refKey` is the referenced column (default `id`). The column and the key are both bigints, or both text: `{"column":"game_type","refTable":"lg_game_family","refKey":"family","relation":"name"}`; the key itself may be the shown column (`"relation":"family"`) when the table has no better one. The shown column may be an integer, such as a player's short id (`"relation":"short_id"`): its options are searched by exact value, which uses its unique index, instead of a `LIKE` over the table; integer columns are never picked by default. The list's search select also offers soft-deleted rows (marked 「（已删除）」, e.g. closed accounts), the form's never does.
   - **Child tables** (`children`): `{"table":"lg_item_i18n","foreignKey":"item_id","keyColumn":"lang","keyDict":"lang","label":"多语言"}` edits a translation table with each row. The form shows one tab per key; rows with all fields empty are not stored. The keys come from the dictionary `keyDict` (its enabled items, maintained in 数据字典), or from a table instead, such as a language registry shared with the game server: `"keyTable":"lg_language","keyValueColumn":"code","keyLabelColumn":"name","keyFilter":"enabled = 1","keySortColumn":"sort","keyRequiredColumn":"required"`. With `keyRequiredColumn`, the rows of the keys marked required must be filled in (the tab shows `*`); `keyFilter` is a fixed SQL condition on that table, run once when generating to check it; it is written into the code as it is, so for a project on both databases use a form both accept (`enabled = TRUE` works for a MySQL `tinyint(1)` and a PostgreSQL `boolean`, `enabled = 1` only on MySQL). The module adds `GET /<resources>/options/<child>Keys` for the form. The detail returns the rows. A save writes the rows of the keys it sends, the form sends the keys it shows: a sent row is replaced (one with every field empty is removed), rows of other keys stay, such as the translations of a language `keyFilter` no longer lists or of a disabled dictionary item; an update without the list keeps every row. The child's key is `(foreignKey, keyColumn)` or its own id (database-assigned or generated). Composite keys of other tables are not supported. Child options: `"fields"` customizes the child's columns like `fields[]` of the main table, with `label`, `form` (`false` leaves out a nullable or defaulted column), `upload` (`"image"`, checked like the main form's images) and `hint`; `"listKeys": true` adds a list column 「已填<label>」 with the keys that have a row (one query per page); `"listKeysLabel"` names that column instead (up to 20 characters), e.g. 「已填语言」.
   - Missing standard columns (`version`, `created_by`, ...) only add a note.
   - **Field options** (`fields[]`, the 字段配置 table on the page): `label`, `list`, `export`, `form`, `search`, `dict`, `upload`, `relation` as above, and
     - `"createOnly": true` (仅新增): set in the create form, read-only when editing, never changed by an update (a game code, a family chosen once);
     - `"hint": "…"` (表单说明, up to 200 characters): a help text under the form input, for what does not fit in the 20-character label;
     - `"list": true` on the primary key: list it, e.g. an order, repair or round number; the key can also be searched (`"search": "eq"`).
   - **Options.** A column comment lists options as `类型：1=通知,2=公告` or `1正常 2冻结 3注销` (with or without a `状态：` in front). `status` is the standard 1 启用 / 2 停用 only when its comment has no numbers; with options in the comment it uses them, with other numbers it is a plain integer, and a dictionary (`fields[].dict`) replaces 启用/停用 too. Yes/no columns (`boolean`, MySQL `tinyint(1)`) take a dictionary when they are not in the form (read-only modules, or `form: false`); its values are `1` and `0`.
   - **Export.** The CSV has the key and the list's columns; `fields[].export` (the 导出 column of the page) adds or removes one. Options and dictionary fields are written as labels, times in UTC with `(UTC)` in the header. An export larger than 基础设置 → 导出行数上限 (default 100,000) is refused with 422 「数据量过大…，请缩小查询范围」, never cut short; the system's own exports (users, logs) follow the same limit.
   - The generated integration test skips when the table is not created by this project's migrations; create the table in the test to run it.

   What the generator supports:
   - **Column types:** text, integer and numeric, boolean, date and time, uuid, json and inet. `*_id` columns and bigint keys become string IDs.
   - **Skipped:** array, bytea and interval columns, and columns whose names look like secrets (password, token, …). The generator prints a note for each.
   - **Other behaviour:**
     - A duplicate value on a column with a unique index is reported on that field ("订单号已存在"), not as a 409.
     - Rows are named by a unique text column in the delete confirmation.
     - The export shows department names instead of IDs.
3. **Restart and grant.** Restart the backend (`go run` does not rebuild on its own). At startup it validates the module, then syncs its permission codes and menus into the database. Grant the new codes to a role in 角色管理 → 分配权限. Super admins have them automatically, and the menu appears for users holding the view code.

   Roles a module needs can be declared in code instead (the generated `module.go` has an empty `Roles()`):
   ```go
   func (m *Module) Roles() []module.RoleDef {
       return []module.RoleDef{
           {Code: "game_ops", Name: "运营", DataScope: "all", Permissions: []string{"game:*"}},
           {Code: "game_cs", Name: "客服", Permissions: []string{"game:player:view", "game:order:*"}},
       }
   }
   ```
   `Permissions` are codes or prefixes ending in `*`; the codes they require (a resource's view code) come along, and startup refuses a pattern that matches no declared code. Startup creates a missing role once, with these permissions, in the same transaction that writes the permission codes, so no migration has to grant codes that do not exist yet. After that administrators own the role: what they rename, grant or take away stays. A later version's new codes that a role's patterns cover are granted on its first start; a code that already existed is not granted again.

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
- **Operation log target:** call `audit.SetTarget(c, "player:"+id)` (`internal/modules/audit`) in a handler to record what the request acted on. 日志审计 → 操作日志 shows it and filters by it exactly, the export includes it. Write it `<kind>:<id>`; a batch call takes several (`SetTarget(c, "player:1", "player:2")`, stored comma-separated). Call it before returning an error too, so failed attempts on the object are found.
- **Integration tests:** use the `integration` build tag with `testinfra/migrated.Database(t)` and `testinfra.RedisDB(t)`, which leases a Redis logical database for the test. To simulate Redis data loss mid-test use `testinfra.FlushRedis(t, client)`, not `FLUSHDB`, which would drop the lease.

### Configuration and extra databases of business modules

The scaffold's configuration is strict (an unknown key stops the start), but it leaves two places to business modules, so they need no change to the core:

- **`biz` section:** anything under `biz:` in the config file is handed to the modules as `cfg.Biz` (`config.Biz`). A module decodes its part, strictly, into its own struct:

  ```yaml
  biz:
    game:
      grpc_addr: game-server:9000
      timeout: 3s
  ```

  ```go
  var gc struct {
      GRPCAddr   string        `mapstructure:"grpc_addr"`
      InnerToken string        `mapstructure:"inner_token"`
      Timeout    time.Duration `mapstructure:"timeout"`
  }
  err := cfg.Biz.Decode("game", &gc)
  ```

  Secrets come from the environment: `ADMIN_BIZ_<KEY>` sets `biz.<key>`, a double underscore separates levels (`ADMIN_BIZ_GAME__INNER_TOKEN` → `biz.game.inner_token`), and environment values override the file. Keys named `password`, `secret` or `token`, or ending with `_password`, `_secret` or `_token`, are refused in the file.
- **Extra databases:** `ADMIN_DATABASE_EXTRA_<NAME>_URL` (postgres:// or mysql://) declares the database `<name>`; pool settings go in the file under `database.extra.<name>` (`max_open_conns` 10, `max_idle_conns` 5, `conn_max_lifetime` 30m, `conn_max_idle_time` 5m, `connect_timeout` 5s, `required` false). Each has its own pool and metrics (`db_name="extra_<name>"`). The connection keeps the server's session defaults: none of the main database's settings (UTC, sql_mode, isolation level) apply, and MySQL URL parameters (`loc`, `tls`, ...) go to the driver; `parseTime=true` is added unless the URL sets it. With `required: true` the API does not start, and `/health/ready` fails, while the database is unreachable; otherwise it starts without it and the module sees the errors. Write that database's own SQL there: the shared-SQL helpers of `internal/platform/database` are for the main database.

Both reach a module in `server/internal/app/modules.go`, where business modules are listed:

```go
game.New(game.Deps{DB: d.db, GameDB: d.extraDB("game"), Biz: cfg.Biz}),
```

A module that asks for an extra database that is not configured stops the start with the variable to set.

## 3. Frontend pages

- **Where views live:** a menu's `component` is a path under `web/src/views` without the extension, e.g. `sales/order/index` → `web/src/views/sales/order/index.vue`. There is no frontend route table to edit: routes come from `/me/menus`.
- **Home page:** after login, `/` and the logo go to `VITE_HOME_PATH` in `web/.env` (a menu path such as `/dashboard/overview`; default the built-in 工作台 `/welcome`). When the user cannot open it, they land on 工作台 (unless `VITE_HIDE_HOME = true`), else on their first menu; with 工作台 hidden, typing `/welcome` goes to the home page too.
- **Button visibility:** gate buttons with `v-perm="'sales:order:create'"` or `hasPerm(...)`. The backend enforces the same codes.
- **Reuse the shared building blocks:** `usePagedTable`, `useFieldErrors`, `confirmAction`, `useDict(code)` (`items`, `labelOf`, `tagTypeOf`), `ReStatusTag`, `formatDateTime`, `formatBytes`, and the shared `http` client (it handles cookies, the XSRF header and the error codes).
- **Menu icons:** pick them in 菜单管理 (about 100 Remix icons, bundled for offline use). To offer more, add names to `web/scripts/gen-menu-icons.mjs`, run it, and format the output. Other Iconify names typed by hand load from the Iconify API.
- **Images and attachments in forms:** `<ReImageUpload v-model="form.coverUrl" />` (one image; store the URL in a `varchar(512)` column) and `<ReFileUpload v-model="form.attachments" :limit="5" />` (a list of `{name, url, size}`; store it in a `jsonb NOT NULL DEFAULT '[]'` column), from `@/components/ReUpload/`. They upload to `POST /files/attachments`, which any signed-in user may call under the upload rules of 基础设置. The returned URL contains a random key and opens for signed-in users only, without the media library permission. On the backend, validate what the client sends with `internal/core/attachment` (`attachment.ValidURL` for one URL, `attachment.Validate` for a list), so only attachment URLs are stored. The notice module is a complete example.
- **Element Plus components:** only the components the project uses are registered, in `web/src/plugins/elementPlus.ts`; `pnpm lint` (`scripts/check-el-components.mjs`) fails when a page or a codegen template uses an `<el-*>` tag that is not registered. Production builds resolve `import { ... } from "element-plus"` to `web/src/plugins/element-plus-entry.ts`, which re-exports from per-component paths (the package's main entry would bundle every component); a value missing there fails the build with "is not exported", so add it there too.
- **Times:** show them with `formatDateTime`. It uses the time zone chosen in the header (the browser's, labelled like `GMT+8`, or UTC; kept in the browser) and appends the label. Send times as UTC RFC 3339 (`toUTC` from `@/utils/timezone`), and put `<ReUtcHint :value="..." />` under date-time pickers to show the UTC of the picked local time.
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

