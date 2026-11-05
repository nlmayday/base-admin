/** 登录页 */
export const LOGIN_PATH = "/login";
/** 强制修改密码页（mustChangePassword 为 true 时只能访问此页） */
export const FORCE_PASSWORD_PATH = "/change-password";
/** 个人中心 */
export const ACCOUNT_PATH = "/account/index";
/** 内置工作台 */
export const WELCOME_PATH = "/welcome";
/** 登录后的默认首页（.env 的 VITE_HOME_PATH，如 /dashboard/overview）；不配置为工作台 */
export const HOME_PATH =
  (import.meta.env.VITE_HOME_PATH ?? "").trim() || WELCOME_PATH;
/** 后端异常时的全屏页 */
export const SERVER_ERROR_PATH = "/server-error";

/** 只允许站内相对路径作为登录后的跳转目标，防止开放重定向 */
export function safeRedirect(value: unknown, fallback = "/"): string {
  if (typeof value !== "string") return fallback;
  if (!value.startsWith("/") || value.startsWith("//")) return fallback;
  if (value.startsWith(LOGIN_PATH) || value.startsWith(FORCE_PASSWORD_PATH)) {
    return fallback;
  }
  return value;
}
