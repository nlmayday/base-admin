import Axios, {
  type AxiosInstance,
  type AxiosResponse,
  type CustomParamsSerializer
} from "axios";
import { stringify } from "qs";
import type {
  ApiEnvelope,
  PureHttpError,
  RequestMethods,
  PureHttpRequestConfig
} from "./types.d";
import { ApiError, ErrorCode, codeFromStatus } from "./errors";
import { handleApiError } from "./handler";

export * from "./errors";
export type { PureHttpRequestConfig } from "./types.d";

/** 接口前缀；开发环境由 Vite 代理到后端，生产环境同源部署 */
export const API_BASE_URL = "/api/v1";

const REQUEST_TIMEOUT_MS = 15000;

/**
 * 会话依赖 HttpOnly Cookie，前端不保存任何凭据。
 * 不安全方法（POST/PUT/PATCH/DELETE）由 axios 自动把 `XSRF-TOKEN` Cookie 写入 `X-XSRF-TOKEN` 请求头。
 */
const instance: AxiosInstance = Axios.create({
  baseURL: API_BASE_URL,
  timeout: REQUEST_TIMEOUT_MS,
  withCredentials: true,
  xsrfCookieName: "XSRF-TOKEN",
  xsrfHeaderName: "X-XSRF-TOKEN",
  headers: {
    Accept: "application/json, text/plain, */*",
    "X-Requested-With": "XMLHttpRequest"
  },
  // 数组参数序列化为 a=1&a=2（https://github.com/axios/axios/issues/5142）
  paramsSerializer: {
    serialize: ((params: Record<string, unknown>) =>
      stringify(params, {
        arrayFormat: "repeat",
        skipNulls: true
      })) as unknown as CustomParamsSerializer
  }
});

function isEnvelope(body: unknown): body is ApiEnvelope {
  return (
    typeof body === "object" &&
    body !== null &&
    typeof (body as ApiEnvelope).code === "number"
  );
}

/** 下载接口出错时响应体是 Blob，需要先解析成 JSON 信封 */
async function readErrorBody(data: unknown): Promise<unknown> {
  if (typeof Blob !== "undefined" && data instanceof Blob) {
    try {
      return JSON.parse(await data.text());
    } catch {
      return null;
    }
  }
  return data;
}

async function toApiError(error: PureHttpError): Promise<ApiError> {
  const response = error.response;
  if (!response) {
    const timedOut = error.code === "ECONNABORTED";
    return new ApiError({
      code: ErrorCode.NETWORK,
      message: timedOut ? "请求超时，请稍后重试" : "网络异常，请检查网络连接"
    });
  }
  const body = await readErrorBody(response.data);
  if (isEnvelope(body)) {
    return new ApiError({
      code: body.code,
      message: body.message,
      httpStatus: response.status,
      data: body.data,
      requestId: body.requestId
    });
  }
  return new ApiError({
    code: codeFromStatus(response.status),
    message: `请求失败（HTTP ${response.status}）`,
    httpStatus: response.status
  });
}

instance.interceptors.response.use(
  (response: AxiosResponse) => {
    // 文件下载：直接返回原始响应，由 download() 处理
    if (response.config.responseType === "blob") return response;
    const body: unknown = response.data;
    if (!isEnvelope(body)) return body as never;
    if (body.code === ErrorCode.OK) return body.data as never;
    // 正常情况下非 0 的 code 都伴随非 2xx 状态码，这里做兜底
    const err = new ApiError({
      code: body.code,
      message: body.message,
      httpStatus: response.status,
      data: body.data,
      requestId: body.requestId
    });
    handleApiError(err, response.config as PureHttpRequestConfig);
    return Promise.reject(err);
  },
  async (error: PureHttpError) => {
    // 主动取消的请求原样抛出，调用方可用 Axios.isCancel 判断
    if (Axios.isCancel(error)) return Promise.reject(error);
    const apiError = await toApiError(error);
    handleApiError(apiError, (error.config ?? {}) as PureHttpRequestConfig);
    return Promise.reject(apiError);
  }
);

/** 从 Content-Disposition 中解析文件名 */
function parseFilename(disposition: string | undefined): string | null {
  if (!disposition) return null;
  const encoded = /filename\*\s*=\s*(?:UTF-8'')?([^;]+)/i.exec(disposition);
  if (encoded) {
    try {
      return decodeURIComponent(encoded[1].trim().replace(/^"|"$/g, ""));
    } catch {
      // 继续尝试普通 filename
    }
  }
  const plain = /filename\s*=\s*"?([^";]+)"?/i.exec(disposition);
  return plain ? plain[1].trim() : null;
}

function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.style.display = "none";
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

class PureHttp {
  /**
   * 通用请求：成功时返回信封中的 `data`，失败时抛出 `ApiError`。
   * 全局错误处理（提示、401 跳转等）已在拦截器中完成，调用方只需处理自己关心的错误码。
   */
  public request<T>(
    method: RequestMethods,
    url: string,
    param?: PureHttpRequestConfig,
    axiosConfig?: PureHttpRequestConfig
  ): Promise<T> {
    // 响应拦截器已把信封解包为 data，axios 的类型推不出这一点
    return instance.request<unknown, T>({
      method,
      url,
      ...param,
      ...axiosConfig
    }) as Promise<T>;
  }

  public get<T>(
    url: string,
    params?: object,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("get", url, { params }, config);
  }

  public post<T>(
    url: string,
    data?: unknown,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("post", url, { data }, config);
  }

  public put<T>(
    url: string,
    data?: unknown,
    config?: PureHttpRequestConfig
  ): Promise<T> {
    return this.request<T>("put", url, { data }, config);
  }

  public delete<T>(url: string, config?: PureHttpRequestConfig): Promise<T> {
    return this.request<T>("delete", url, {}, config);
  }

  /** 以文件形式下载 GET 接口（如 CSV 导出），文件名优先取 Content-Disposition */
  public async download(
    url: string,
    params: object,
    fallbackFilename: string
  ): Promise<void> {
    const response = await instance.get<Blob>(url, {
      params,
      responseType: "blob",
      timeout: 0
    });
    const filename =
      parseFilename(response.headers["content-disposition"] as string) ??
      fallbackFilename;
    saveBlob(response.data, filename);
  }
}

export const http = new PureHttp();
