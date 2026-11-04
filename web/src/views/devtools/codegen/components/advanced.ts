import type { CodegenChild, CodegenKeyMode } from "@/api/devtools/codegen";

/** 手动声明的关联：本表的 bigint 列 → 关联表.显示列（不需要外键约束） */
export type DeclaredRef = {
  column: string;
  refTable: string;
  refKey: string;
  relation: string;
};

/** 子表配置；keySource 只在页面上用，决定键来自字典还是表 */
export type ChildModel = CodegenChild & { keySource: "dict" | "table" };

export type AdvancedModel = {
  keyMode: CodegenKeyMode;
  readOnly: boolean;
  refs: DeclaredRef[];
  children: ChildModel[];
};

/** 提交给后端的子表配置：只保留所选键来源的项，去掉空值 */
export function childSpecs(children: ChildModel[]): CodegenChild[] {
  return children
    .filter(c => c.table)
    .map(({ keySource, ...c }) => {
      const out: CodegenChild = {
        table: c.table.trim(),
        foreignKey: c.foreignKey.trim(),
        keyColumn: c.keyColumn.trim()
      };
      if (c.label?.trim()) out.label = c.label.trim();
      if (keySource === "dict") {
        out.keyDict = c.keyDict;
        return out;
      }
      for (const k of [
        "keyTable",
        "keyValueColumn",
        "keyLabelColumn",
        "keyFilter",
        "keySortColumn",
        "keyRequiredColumn"
      ] as const) {
        const v = c[k]?.trim();
        if (v) out[k] = v;
      }
      return out;
    });
}

/** 后端的子表配置转回页面模型 */
export function childModels(children: CodegenChild[]): ChildModel[] {
  return children.map(c => ({
    ...c,
    keySource: c.keyTable ? "table" : "dict"
  }));
}
