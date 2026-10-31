/** 后端错误码（见 docs/api-p0.md「Error codes」） */
export const ErrorCode = {
  OK: 0,
  /** 前端自定义：网络错误 / 无响应 */
  NETWORK: -1,
  /** 前端自定义：响应不是约定的信封格式 */
  UNKNOWN: -2,
  VALIDATION: 10002,
  NOT_FOUND: 10003,
  CONFLICT: 10004,
  RATE_LIMITED: 10005,
  UNAVAILABLE: 10006,
  OUTCOME_UNKNOWN: 10008,
  UNAUTHENTICATED: 20001,
  FORBIDDEN: 20002,
  CSRF_REJECTED: 20003,
  BAD_CREDENTIALS: 20004,
  PASSWORD_CHANGE_REQUIRED: 20005,
  ACCOUNT_LOCKED: 20006,
  CAPTCHA_REQUIRED: 20007,
  PERMISSION_DEPENDENCY: 30001,
  PROTECTED_RESOURCE: 30002,
  RESOURCE_IN_USE: 30003,
  PROTECTED_CONFIG: 50001
} as const;

export type FieldError = { field: string; message: string };

export type PermissionDependencyData = {
  missing: string[];
  unknown: string[];
};

type ApiErrorInit = {
  code: number;
  message: string;
  httpStatus?: number;
  data?: unknown;
  requestId?: string;
};

/** 统一的接口错误对象：拦截器把所有失败（含网络错误）都转换成它 */
export class ApiError extends Error {
  readonly code: number;
  readonly httpStatus: number;
  readonly data: unknown;
  readonly requestId: string;

  constructor(init: ApiErrorInit) {
    super(init.message);
    this.name = "ApiError";
    this.code = init.code;
    this.httpStatus = init.httpStatus ?? 0;
    this.data = init.data ?? null;
    this.requestId = init.requestId ?? "";
  }

  /** 422 / 10002 时的字段错误 */
  get fields(): FieldError[] {
    const fields = (this.data as { fields?: unknown } | null)?.fields;
    if (!Array.isArray(fields)) return [];
    return fields.filter(
      (f): f is FieldError =>
        typeof f?.field === "string" && typeof f?.message === "string"
    );
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError;
}

export function hasErrorCode(err: unknown, code: number): err is ApiError {
  return isApiError(err) && err.code === code;
}

/** 409 版本冲突或 503 结果未知：调用方应重新加载记录 */
export function needsReload(err: unknown): boolean {
  return (
    hasErrorCode(err, ErrorCode.CONFLICT) ||
    hasErrorCode(err, ErrorCode.OUTCOME_UNKNOWN)
  );
}

/** 按 HTTP 状态码兜底推断错误码（响应体不是信封时使用） */
export function codeFromStatus(status: number): number {
  switch (status) {
    case 401:
      return ErrorCode.UNAUTHENTICATED;
    case 403:
      return ErrorCode.FORBIDDEN;
    case 404:
      return ErrorCode.NOT_FOUND;
    case 409:
      return ErrorCode.CONFLICT;
    case 422:
      return ErrorCode.VALIDATION;
    case 429:
      return ErrorCode.RATE_LIMITED;
    case 503:
      return ErrorCode.UNAVAILABLE;
    default:
      return ErrorCode.UNKNOWN;
  }
}
