# Upstream

This frontend is based on **pure-admin-thin**.

| Item       | Value                                                                |
| ---------- | -------------------------------------------------------------------- |
| Repository | https://github.com/pure-admin/pure-admin-thin                        |
| Tag        | `v6.2.0`                                                             |
| Commit     | `f0ff132561ab684bb78379239adf29f9a38ac7f1` (2025-10-30)              |
| License    | MIT, Copyright (c) 2020-present, pure-admin (see [LICENSE](LICENSE)) |
| Imported   | 2026-10-30, `.git` removed, imported as plain files                  |

## Local changes from upstream

- **Auth**: removed token storage/refresh (`src/utils/auth.ts`, the refresh flow in `src/utils/http`, the `multiple-tabs` cookie, `src/utils/sso.ts`). The backend uses an HttpOnly session cookie plus `XSRF-TOKEN`; axios runs with `withCredentials` and the XSRF cookie/header names. The response envelope is unwrapped and error codes are handled in `src/utils/http/handler.ts`.
- **Routes**: the guard in `src/router/index.ts` restores the session with `GET /me` and loads dynamic routes from `GET /me/menus` on every login and page refresh. Routes are no longer cached in localStorage, and nothing is filtered by role on the frontend. `src/router/utils.ts` resolves `component` to the exact `src/views/<component>.vue` path and maps external-link menus (`path` = URL) to the pure-admin convention (`name` = URL).
- **Permissions**: there is a single entry point, `hasPerm()` in `src/utils/permission.ts`, plus the `v-perm` directive. The template's `hasAuth`, `hasPerms`, `v-auth`, `v-perms`, `<Auth>` and `<Perms>` were removed.
- **Removed**: `mock/` and `vite-plugin-fake-server`, `@faker-js/faker`, `js-cookie`, the demo `permission` views, the "异常页面" demo menu, and the notice bell (`lay-notice`, which showed fake data).
- **Added**: `src/api/{auth,iam}`, `src/views/system/*`, `src/views/account/*`, `src/composables/*`, and Playwright E2E tests in `tests/e2e`.
- **Vite**: `/api` is proxied to `VITE_API_PROXY_TARGET` (default `http://127.0.0.1:8080`).

When updating from upstream, diff against the tag above and reapply these changes.
