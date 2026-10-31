import type { TreeOptionProps } from "element-plus/es/components/tree/src/tree.type";

/** 树形数据的通用工具（部门、菜单） */
export type TreeLike<T> = T & { children?: TreeLike<T>[] | null };

/** 收集某节点及其全部后代的 id，用于禁止把节点移动到自己或后代下面 */
export function collectSubtreeIds<T extends { id: string }>(
  nodes: TreeLike<T>[],
  rootId: string
): Set<string> {
  const ids = new Set<string>();
  const visit = (list: TreeLike<T>[], inside: boolean): void => {
    for (const node of list ?? []) {
      const hit = inside || node.id === rootId;
      if (hit) ids.add(node.id);
      visit(node.children ?? [], hit);
    }
  };
  visit(nodes, false);
  return ids;
}

/** 返回把 disabledIds 中的节点标记为 disabled 的新树（不修改原树） */
export function markDisabled<T extends { id: string }>(
  nodes: TreeLike<T>[],
  disabledIds: ReadonlySet<string>
): TreeLike<T & { disabled: boolean }>[] {
  return (nodes ?? []).map(node => ({
    ...node,
    disabled: disabledIds.has(node.id),
    children: markDisabled(node.children ?? [], disabledIds)
  }));
}

/** 广度优先展开为平铺列表 */
export function flattenTree<T>(nodes: TreeLike<T>[]): TreeLike<T>[] {
  const out: TreeLike<T>[] = [];
  const queue = [...(nodes ?? [])];
  while (queue.length) {
    const node = queue.shift()!;
    out.push(node);
    queue.push(...(node.children ?? []));
  }
  return out;
}

/**
 * el-tree-select 的 props。运行时用 value 键取节点的值，但 Element Plus 2.14
 * 的 TreeOptionProps 类型里漏了 value，这里集中做一次类型断言。
 */
export function treeSelectProps(
  label = "name",
  extra: { disabled?: string } = {}
): TreeOptionProps {
  return {
    value: "id",
    label,
    children: "children",
    ...extra
  } as TreeOptionProps;
}

/** 部门树选择（id / name / children） */
export const DEPT_TREE_PROPS = treeSelectProps("name");
