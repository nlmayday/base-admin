import { message } from "@/utils/message";
import { type ApiError, ErrorCode } from "./errors";
import type { PureHttpRequestConfig } from "./types.d";
import { router } from "@/router";
import { LOGIN_PATH, FORCE_PASSWORD_PATH } from "@/router/paths";
import { useUserStoreHook } from "@/store/modules/user";

/** 由调用方（表单、授权抽屉、登录页）自行展示的错误码，拦截器不弹提示 */
const CALLER_HANDLED_CODES = new Set<number>([
  ErrorCode.BAD_CREDENTIALS,
  ErrorCode.ACCOUNT_LOCKED,
  ErrorCode.LOGIN_IP_DENIED,
  ErrorCode.LOGIN_TIME_DENIED,
  ErrorCode.LOGIN_DENIED,
  ErrorCode.CAPTCHA_REQUIRED,
  ErrorCode.PERMISSION_DEPENDENCY
]);

const CSRF_RELOAD_DELAY_MS = 1500;

let redirectingToLogin = false;

function currentFullPath(): string {
  return router.currentRoute.value.fullPath;
}

function onUnauthenticated(): void {
  useUserStoreHook().clearSession();
  const route = router.currentRoute.value;
  if (route.path === LOGIN_PATH || redirectingToLogin) return;
  redirectingToLogin = true;
  message("登录状态已失效，请重新登录", { type: "warning" });
  router
    .replace({ path: LOGIN_PATH, query: { redirect: currentFullPath() } })
    .finally(() => {
      redirectingToLogin = false;
    });
}

function onPasswordChangeRequired(): void {
  useUserStoreHook().markMustChangePassword();
  if (router.currentRoute.value.path === FORCE_PASSWORD_PATH) return;
  message("请先修改密码", { type: "warning" });
  router.replace({
    path: FORCE_PASSWORD_PATH,
    query: { redirect: currentFullPath() }
  });
}

/** 全局错误处理：按 docs/api-p0.md 的错误码表执行前端行为 */
export function handleApiError(
  err: ApiError,
  config: PureHttpRequestConfig
): void {
  if (config.skipErrorHandler) return;
  if (CALLER_HANDLED_CODES.has(err.code)) return;

  switch (err.code) {
    case ErrorCode.UNAUTHENTICATED:
      onUnauthenticated();
      return;
    case ErrorCode.FORBIDDEN:
      message("无权限", { type: "error" });
      return;
    case ErrorCode.CSRF_REJECTED:
      message(err.message || "安全校验失败，页面即将刷新", { type: "error" });
      setTimeout(() => window.location.reload(), CSRF_RELOAD_DELAY_MS);
      return;
    case ErrorCode.PASSWORD_CHANGE_REQUIRED:
      onPasswordChangeRequired();
      return;
    case ErrorCode.CONFLICT:
      message("数据已被他人修改，请刷新", { type: "warning" });
      return;
    case ErrorCode.OUTCOME_UNKNOWN:
      message("结果未知，请刷新确认", { type: "warning" });
      return;
    case ErrorCode.VALIDATION:
      message(
        err.fields.length > 0
          ? "提交的数据有误，请检查表单"
          : err.message || "提交的数据有误",
        { type: "error" }
      );
      return;
    default:
      message(err.message || "请求失败", { type: "error" });
  }
}
