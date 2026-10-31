import type { Method, AxiosError, AxiosRequestConfig } from "axios";

export type RequestMethods = Extract<
  Method,
  "get" | "post" | "put" | "delete" | "patch"
>;

/** 后端统一响应信封 */
export type ApiEnvelope<T = unknown> = {
  code: number;
  message: string;
  data: T;
  requestId: string;
};

export interface PureHttpError extends AxiosError {
  isCancelRequest?: boolean;
}

export interface PureHttpRequestConfig extends AxiosRequestConfig {
  /**
   * 跳过全局错误处理（提示、401 跳转登录等），由调用方自行处理。
   * 用于路由守卫中的会话探测等场景。
   */
  skipErrorHandler?: boolean;
}
