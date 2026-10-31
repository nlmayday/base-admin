# P0 API contract

This is the HTTP contract between the backend (`server/`) and the frontend (`web/`) for P0. The architectural rules live in [rebuild-plan-v2.md](rebuild-plan-v2.md); this document only fixes request and response shapes.

## Conventions

- Base path `/api/v1`. The frontend is served from the same origin; in development Vite proxies `/api` to `http://127.0.0.1:8080`.
- Every JSON response uses this envelope: `{ "code": 0, "message": "success", "data": …, "requestId": "req_…" }`. `code != 0` means an error; the HTTP status follows doc section 9.
- **IDs are strings** in JSON, in both requests and responses. Times are RFC 3339 strings.
- Paged lists take `?page=1&pageSize=20` (max 100) and return `data = { list, total, page, pageSize }`.
- Editable entities carry `version` (integer). Send it back on update; a stale version returns **409** with code `10004`.
- Status values: `1` = enabled, `2` = disabled.
- `message` and validation `fields[].message` are in Chinese and can be shown to users as is.
- Permission lists (`/permissions/tree`, `/menus/tree`, `/me`) follow module declaration order, so a resource's `view` code comes first.

### Authentication and CSRF

- Login sets two cookies:
  - The session cookie (`sid`, or `__Host-sid` in production), HttpOnly. JavaScript never sees it.
  - `XSRF-TOKEN`, readable by JavaScript.
- For every `POST/PUT/PATCH/DELETE`, send the `XSRF-TOKEN` cookie value in the `X-XSRF-TOKEN` header. Axios does this automatically when `xsrfCookieName: "XSRF-TOKEN"`, `xsrfHeaderName: "X-XSRF-TOKEN"` and `withCredentials: true` are set.
- The browser must send `Origin` on unsafe requests (browsers always do). Origins that are not allowed get 403 / `20003`.
- No token is stored in `localStorage`, and there is no refresh endpoint. Sessions slide on activity: 2 h idle, 12 h absolute.
- `POST /auth/login` needs a valid `Origin` but no XSRF header unless a session cookie already exists. Logging in destroys any session the browser presented, which prevents session fixation.
- Non-super-admins cannot grant permission codes they do not hold. They also cannot assign a role whose codes exceed theirs, or create or assign an `all`-scope role unless they already have that scope. These attempts return 403 / `20002`.

### Error codes

| code | HTTP | meaning | frontend behaviour |
|---|---|---|---|
| 10002 | 422 | validation failed; `data.fields = [{field, message}]` | show field errors |
| 10003 | 404 | not found, or outside the data scope | toast |
| 10004 | 409 | version or unique conflict | "数据已被他人修改，请刷新" |
| 10005 | 429 | rate limited | toast |
| 10006 | 503 | dependency unavailable | toast |
| 10008 | 503 | outcome unknown (connection lost during commit) | "结果未知，请刷新确认" and reload the record |
| 20001 | 401 | not logged in / session expired or revoked | clear user state, go to `/login?redirect=…` |
| 20002 | 403 | missing permission | toast "无权限" (no redirect) |
| 20003 | 403 | CSRF / origin check failed | toast, then reload the page |
| 20004 | 401 | wrong username or password (login only) | show on the login form |
| 20005 | 403 | password change required | go to the change-password page |
| 20006 | 403 | account locked after repeated failures; `data.retryAfterSeconds` | show on the login form |
| 20007 | 401 | captcha missing or wrong; `data.captchaRequired` | show the captcha |
| 20008 | 403 | the security policy's IP allowlist refuses this IP (checked before the password) | show on the login form |
| 20009 | 403 | outside the security policy's login window; `data = {start, end, timeZone}` | show on the login form |
| 20010 | 403 | a business module's `LoginGuard` refused the login | show on the login form |
| 30001 | 422 | permission grant violates dependencies; `data = {missing: [], unknown: []}` | highlight missing codes |
| 30002 | 409 | protected resource (built-in role, last super admin, yourself) | toast message |
| 30003 | 409 | resource in use (department has children or users, role or position assigned) | toast message |

## Auth (module `auth`)

| method | path | access | request | response `data` |
|---|---|---|---|---|
| POST | /auth/login | public, rate limited | `{username, password}` | `Me` |
| POST | /auth/logout | authenticated | – | `null` (cookies cleared) |
| GET | /me | authenticated | – | `Me` |
| GET | /me/menus | authenticated | – | `AsyncRoute[]` |
| PUT | /me/password | authenticated | `{oldPassword, newPassword}` | `null`; a new session cookie is issued and other devices are logged out |
| PUT | /me/profile | authenticated | `{nickname, email, phone, avatarUrl, version}` | `Me` |

```ts
type Me = {
  user: { id: string; username: string; nickname: string; email: string; phone: string;
          avatarUrl: string; deptId: string | null; deptName: string; version: number };
  roles: { id: string; code: string; name: string }[];
  permissions: string[];          // super admin gets every code expanded
  isSuperAdmin: boolean;
  mustChangePassword: boolean;    // when true only /me, /me/password, /auth/logout work (20005 otherwise)
};
```

`/me/menus` returns pure-admin async routes. Only menus the user may view are included, and empty directories are dropped.

```ts
type AsyncRoute = {
  path: string;                 // "/system" for a directory, "/system/user" for a page, a URL for an external link
  name?: string;                // unique, PascalCase of the menu code, e.g. "IamUser"
  component?: string;           // view path under src/views without extension, e.g. "system/user/index"
  meta: { title: string; icon?: string; rank?: number; showLink?: boolean; keepAlive?: boolean };
  children?: AsyncRoute[];
};
```

## IAM (module `iam`)

### Permission codes

| resource | codes |
|---|---|
| user | `iam:user:view` `create` `update` `delete` `reset-password` `export` |
| role | `iam:role:view` `create` `update` `delete` `grant` |
| dept | `iam:dept:view` `create` `update` `delete` |
| position | `iam:position:view` `create` `update` `delete` |
| menu | `iam:menu:view` `create` `update` `delete` |
| api | `iam:api:view` |

Every non-view code requires the `view` code of its resource; no other dependencies exist in P0.

### Menus provided

These sit under the directory "系统管理" (`/system`):

| menu code | path | component | permission |
|---|---|---|---|
| iam.user | /system/user | system/user/index | iam:user:view |
| iam.role | /system/role | system/role/index | iam:role:view |
| iam.dept | /system/dept | system/dept/index | iam:dept:view |
| iam.position | /system/position | system/position/index | iam:position:view |
| iam.menu | /system/menu | system/menu/index | iam:menu:view |
| iam.api | /system/api | system/api/index | iam:api:view |

### Users

```ts
type User = { id: string; username: string; nickname: string; email: string; phone: string; avatarUrl: string;
  deptId: string | null; deptName: string; status: 1 | 2; roleIds: string[]; positionIds: string[];
  lastLoginAt: string | null; createdAt: string; updatedAt: string; version: number };
```

| method | path | access | request | response `data` |
|---|---|---|---|---|
| GET | /users | iam:user:view | `?page&pageSize&keyword&deptId&status` | `Page<User>` |
| GET | /users/:id | iam:user:view | – | `User` |
| POST | /users | iam:user:create | `{username, password, nickname, email, phone, deptId, status, roleIds, positionIds}` | `User` |
| PUT | /users/:id | iam:user:update | `{nickname, email, phone, deptId, roleIds, positionIds, version}` | `User` |
| PUT | /users/:id/status | iam:user:update | `{status, version}` | `User` |
| PUT | /users/:id/password | iam:user:reset-password | `{password}` | `null` (all of that user's sessions end) |
| DELETE | /users/:id | iam:user:delete | – | `null` |
| GET | /users/options | authenticated | `?keyword` | `{id, username, nickname}[]` (max 50, within the data scope) |
| GET | /users/export | iam:user:export | same filters as list | `text/csv` download |

Rules:
- Usernames are 3–32 characters from `[a-zA-Z0-9_.-]` and are unique, case-insensitive.
- Passwords are 8–72 characters.
- You cannot disable or delete yourself.
- The last enabled super admin cannot be disabled, deleted or stripped of the role.
- Only a super admin can assign `super_admin`.

### Roles

```ts
type Role = { id: string; code: string; name: string;
  dataScope: "all" | "dept_and_children" | "dept" | "self" | "custom"; deptIds: string[];
  status: 1 | 2; builtin: boolean; sort: number; remark: string; createdAt: string; version: number };
```

| method | path | access | request | response `data` |
|---|---|---|---|---|
| GET | /roles | iam:role:view | `?page&pageSize&keyword` | `Page<Role>` |
| GET | /roles/options | authenticated | – | `{id, code, name}[]` (enabled roles) |
| POST | /roles | iam:role:create | `{code, name, dataScope, deptIds, status, sort, remark}` | `Role` |
| PUT | /roles/:id | iam:role:update | `{name, dataScope, deptIds, status, sort, remark, version}` | `Role` |
| DELETE | /roles/:id | iam:role:delete | – | `null` (30003 if assigned) |
| GET | /roles/:id/permissions | iam:role:grant | – | `{codes: string[]}` |
| PUT | /roles/:id/permissions | iam:role:grant | `{codes: string[]}` | `{codes}` (30001 on dependency violations) |
| GET | /permissions/tree | iam:role:grant | – | `PermNode[]` |

```ts
type PermNode = { menuCode: string; title: string; type: "dir" | "menu" | "link";
  permissions: { code: string; name: string; requires: string[] }[]; children: PermNode[] };
```

`dataScope` defaults to `self`. `deptIds` is used only with `custom`. The built-in role `super_admin` cannot be edited, granted or deleted (30002). Role codes match `^[a-z][a-z0-9_]{1,31}$`.

### Departments

```ts
type Dept = { id: string; parentId: string | null; name: string; leaderUserId: string | null;
  sort: number; status: 1 | 2; version: number; children: Dept[] };
```

| method | path | access | request | response `data` |
|---|---|---|---|---|
| GET | /departments/tree | any of iam:dept:view, iam:user:view | – | `Dept[]` (within the data scope) |
| POST | /departments | iam:dept:create | `{parentId, name, leaderUserId, sort, status}` | `Dept` |
| PUT | /departments/:id | iam:dept:update | `{parentId, name, leaderUserId, sort, status, version}` | `Dept` (moving under itself or a descendant gives 422) |
| DELETE | /departments/:id | iam:dept:delete | – | `null` (30003 if it has children or users) |

### Positions

```ts
type Position = { id: string; code: string; name: string; sort: number; status: 1 | 2; remark: string; version: number };
```

| method | path | access | request | response `data` |
|---|---|---|---|---|
| GET | /positions | iam:position:view | `?page&pageSize&keyword` | `Page<Position>` |
| GET | /positions/options | authenticated | – | `{id, code, name}[]` |
| POST | /positions | iam:position:create | `{code, name, sort, status, remark}` | `Position` |
| PUT | /positions/:id | iam:position:update | `{name, sort, status, remark, version}` | `Position` |
| DELETE | /positions/:id | iam:position:delete | – | `null` (30003 if assigned) |

### Menus

```ts
type MenuNode = { id: string; code: string; parentId: string | null; type: "dir" | "menu" | "link";
  title: string; path: string; component: string; icon: string; sort: number; hidden: boolean;
  keepAlive: boolean; permissionCode: string; externalUrl: string; source: "module" | "custom";
  version: number;
  permissions: { code: string; name: string }[];  // read-only button codes under this menu, in declaration order
  children: MenuNode[] };
```

| method | path | access | request | response `data` |
|---|---|---|---|---|
| GET | /menus/tree | iam:menu:view | – | `MenuNode[]` |
| POST | /menus | iam:menu:create | `{parentId, type: "dir" or "link", title, path, icon, sort, hidden, externalUrl}` | `MenuNode` |
| PUT | /menus/:id | iam:menu:update | `{title, icon, sort, hidden, version}`, plus `parentId, path, externalUrl` for custom menus | `MenuNode` |
| DELETE | /menus/:id | iam:menu:delete | – | `null` (module menus: 30002) |

Module menus (`source = "module"`) can only change `title`, `icon`, `sort` and `hidden`. A custom `dir` needs a `path` starting with `/`; a custom `link` needs an `http(s)` `externalUrl`. In `/me/menus` the link's `path` is that URL, and the frontend maps it to pure-admin's external-link convention.

### API routes (read-only)

| method | path | access | response `data` |
|---|---|---|---|
| GET | /api-routes | iam:api:view | `{method, path, access, kind, permissions, module}[]`; `kind` is `public`, `authenticated` or `permission`, and `permissions` is `{code, name}[]`, any of which grants access |
