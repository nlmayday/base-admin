import type { AccessKind, RouteDoc } from "@/api/devtools/apidoc";

export type TagType = "success" | "primary" | "warning" | "danger" | "info";

export const METHOD_TAG: Record<string, TagType> = {
  GET: "success",
  POST: "primary",
  PUT: "warning",
  PATCH: "warning",
  DELETE: "danger"
};

export function methodTag(method: string): TagType {
  return METHOD_TAG[method.toUpperCase()] ?? "info";
}

export const ACCESS_META: Record<
  Exclude<AccessKind, "permission">,
  { label: string; tag: TagType; tip: string }
> = {
  public: { label: "公开", tag: "warning", tip: "无需登录即可访问" },
  authenticated: {
    label: "登录即可",
    tag: "info",
    tip: "需要有效会话，不需要权限码"
  }
};

/** 路由唯一键 */
export function routeKey(r: Pick<RouteDoc, "method" | "path">): string {
  return `${r.method} ${r.path}`;
}

/** 是否匹配搜索词：路径、摘要、权限码与权限名称 */
export function matchesKeyword(r: RouteDoc, keyword: string): boolean {
  const kw = keyword.trim().toLowerCase();
  if (!kw) return true;
  return (
    r.path.toLowerCase().includes(kw) ||
    r.summary.toLowerCase().includes(kw) ||
    r.method.toLowerCase() === kw ||
    r.permissions.some(
      p => p.code.toLowerCase().includes(kw) || p.name.includes(kw)
    )
  );
}

/** 请求地址示例：路径参数用 1 代替，查询参数列出名称 */
export function exampleURL(r: RouteDoc): string {
  const path = r.path.replace(/[:*]([A-Za-z_]\w*)/g, (_m, name: string) =>
    /id$/i.test(name) ? "1" : `{${name}}`
  );
  const query = r.parameters
    .filter(p => p.in === "query")
    .map(p => `${p.name}=`)
    .join("&");
  return query ? `${path}?${query}` : path;
}

/** 示例 JSON 文本；没有示例时返回空串 */
export function toJSON(value: unknown): string {
  if (value === null || value === undefined) return "";
  return JSON.stringify(value, null, 2);
}
