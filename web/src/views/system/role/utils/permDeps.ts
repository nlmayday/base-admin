import type { PermNode, PermissionDef } from "@/api/iam/role";

/** code → 直接依赖的权限码 */
export type RequiresMap = ReadonlyMap<string, readonly string[]>;

/** 收集权限树中的全部权限定义 */
export function collectPermissions(tree: readonly PermNode[]): PermissionDef[] {
  const out: PermissionDef[] = [];
  const visit = (nodes: readonly PermNode[]): void => {
    for (const node of nodes ?? []) {
      out.push(...(node.permissions ?? []));
      visit(node.children ?? []);
    }
  };
  visit(tree);
  return out;
}

export function buildRequiresMap(tree: readonly PermNode[]): RequiresMap {
  return new Map(
    collectPermissions(tree).map(p => [p.code, [...(p.requires ?? [])]])
  );
}

/** code 依赖的全部权限码（传递闭包，不含自身） */
export function requiresClosure(code: string, map: RequiresMap): Set<string> {
  const result = new Set<string>();
  const stack = [...(map.get(code) ?? [])];
  while (stack.length) {
    const next = stack.pop()!;
    if (result.has(next) || next === code) continue;
    result.add(next);
    stack.push(...(map.get(next) ?? []));
  }
  return result;
}

/** 依赖 code 的全部权限码（反向传递闭包，不含自身） */
export function dependentsClosure(code: string, map: RequiresMap): Set<string> {
  const result = new Set<string>();
  for (const candidate of map.keys()) {
    if (candidate !== code && requiresClosure(candidate, map).has(code)) {
      result.add(candidate);
    }
  }
  return result;
}

/**
 * 勾选：同时勾选它依赖的权限；取消：同时取消依赖它的权限。
 * 返回新的集合，不修改入参。最终提交的就是界面上勾选的集合。
 */
export function togglePermission(
  checked: ReadonlySet<string>,
  code: string,
  on: boolean,
  map: RequiresMap
): Set<string> {
  const next = new Set(checked);
  if (on) {
    next.add(code);
    requiresClosure(code, map).forEach(c => next.add(c));
  } else {
    next.delete(code);
    dependentsClosure(code, map).forEach(c => next.delete(c));
  }
  return next;
}

/** 节点及其子孙节点上的全部权限码 */
export function subtreeCodes(node: PermNode): string[] {
  return collectPermissions([node]).map(p => p.code);
}

/** 批量勾选或取消，依赖规则与单个勾选一致 */
export function toggleMany(
  checked: ReadonlySet<string>,
  codes: readonly string[],
  on: boolean,
  map: RequiresMap
): Set<string> {
  return codes.reduce<Set<string>>(
    (acc, code) => togglePermission(acc, code, on, map),
    new Set(checked)
  );
}

export type GroupState = "all" | "some" | "none";

/** 一组权限码的勾选状态，用于全选框的选中/半选 */
export function groupState(
  checked: ReadonlySet<string>,
  codes: readonly string[]
): GroupState {
  const n = codes.filter(c => checked.has(c)).length;
  if (n === 0) return "none";
  return n === codes.length ? "all" : "some";
}

export interface PermRow {
  node: PermNode;
  depth: number;
  codes: string[];
}

/** 把权限树展开为按层级缩进的行 */
export function flattenRows(tree: readonly PermNode[], depth = 0): PermRow[] {
  return tree.flatMap(node => [
    { node, depth, codes: subtreeCodes(node) },
    ...flattenRows(node.children ?? [], depth + 1)
  ]);
}
