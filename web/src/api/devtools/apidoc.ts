import { http } from "@/utils/http";

/** 访问级别：公开 / 登录即可 / 需要权限码 */
export type AccessKind = "public" | "authenticated" | "permission";

/** 结构树中的一个字段（对象属性、数组元素或单值） */
export type SchemaField = {
  name: string;
  /** 类型描述，如 string(date-time)、array<iam.userView> */
  type: string;
  required: boolean;
  nullable: boolean;
  /** 校验规则，如「长度 ≤ 64」「可选值：1 | 2」 */
  constraints: string[];
  description: string;
  /** 具名结构体的组件名 */
  ref?: string;
  /** 指向外层同类型的循环引用，不再展开 */
  circular?: boolean;
  children?: SchemaField[];
};

export type RouteParameter = {
  name: string;
  in: "path" | "query";
  type: string;
  required: boolean;
  constraints: string[];
  description: string;
};

export type RoutePermission = { code: string; name: string };

export type RouteDoc = {
  method: string;
  /** Gin 路由模板，如 /api/v1/users/:id */
  path: string;
  module: string;
  operationId: string;
  summary: string;
  description: string;
  access: AccessKind;
  /** 拥有其中任意一个即可访问 */
  permissions: RoutePermission[];
  /** 敏感读取（如导出），会写入操作日志 */
  sensitive: boolean;
  /** 是否需要 X-XSRF-TOKEN 请求头 */
  csrf: boolean;
  parameters: RouteParameter[];
  requestBody: SchemaField[] | null;
  requestExample: unknown;
  /** 成功响应的内容类型，如 application/json、text/csv */
  responseType: string;
  /** 信封中 data 的结构 */
  response: SchemaField[] | null;
  responseExample: unknown;
};

export type ModuleSummary = { name: string; title: string; count: number };

export type RoutesDoc = { modules: ModuleSummary[]; routes: RouteDoc[] };

export const getApiDoc = () => http.get<RoutesDoc>("/devtools/routes");

export const downloadOpenAPI = () =>
  http.download("/devtools/openapi.json", {}, "openapi.json");
