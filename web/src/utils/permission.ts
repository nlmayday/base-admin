import { useUserStoreHook } from "@/store/modules/user";

/** 权限码或权限码列表（列表表示需要同时拥有全部权限码） */
export type PermValue = string | readonly string[];

/**
 * 按钮级权限的唯一判断入口。权限码来自 GET /me，保存在 Pinia 中的 Set。
 * 前端隐藏按钮只是体验优化，真正的安全边界在后端。
 */
export function hasPerm(value: PermValue): boolean {
  const codes = typeof value === "string" ? [value] : value;
  if (!codes || codes.length === 0) return false;
  const { permissions } = useUserStoreHook();
  return codes.every(code => permissions.has(code));
}
