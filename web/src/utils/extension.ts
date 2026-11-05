/**
 * 代码生成的列表页读取扩展文件（<Resource>Extension.vue）导出的选项。
 * 扩展文件只生成一次、永不覆盖，旧的扩展文件没有新选项，这里给默认值。
 */
export type ExtensionOptions<T> = {
  /** 有行内扩展按钮（显示「操作」列） */
  hasRowActions: boolean;
  /** 表格上方的说明区（place="header"） */
  hasHeader: boolean;
  /** 展开行（place="expand"） */
  hasExpand: boolean;
  /** 由扩展接管渲染的列（字段名），渲染 place="cell" */
  cellColumns: readonly string[];
  /** 返回 false 时该行不显示「编辑」 */
  canEdit: (row: T) => boolean;
  /** 返回 false 时该行不显示「删除」 */
  canDelete: (row: T) => boolean;
};

const always = (): boolean => true;

export function extensionOptions<T>(mod: unknown): ExtensionOptions<T> {
  const m = (mod ?? {}) as Partial<ExtensionOptions<T>>;
  return {
    hasRowActions: m.hasRowActions === true,
    hasHeader: m.hasHeader === true,
    hasExpand: m.hasExpand === true,
    cellColumns: Array.isArray(m.cellColumns) ? m.cellColumns : [],
    canEdit: typeof m.canEdit === "function" ? m.canEdit : always,
    canDelete: typeof m.canDelete === "function" ? m.canDelete : always
  };
}
