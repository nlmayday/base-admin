import type {
  CodegenChild,
  CodegenFieldOption,
  CodegenKeyMode
} from "@/api/devtools/codegen";

/** 手动声明的关联：本表的 bigint 列 → 关联表.显示列（不需要外键约束） */
export type DeclaredRef = {
  column: string;
  refTable: string;
  refKey: string;
  relation: string;
};

/** 子表的一个字段配置（页面模型） */
export type ChildFieldModel = {
  column: string;
  label: string;
  /** 图片上传（存图片地址） */
  image: boolean;
  /** 不在表单中（可为空或有默认值的列） */
  hidden: boolean;
  hint: string;
  /** 列表增加模糊搜索项 */
  search?: boolean;
  placeholder?: string;
};

/** 子表配置；keySource 只在页面上用，决定键来自字典还是表 */
export type ChildModel = Omit<CodegenChild, "fields" | "requiredFields"> & {
  keySource: "dict" | "table";
  fieldRows: ChildFieldModel[];
  /** 必填字段，逗号分隔 */
  requiredText?: string;
};

/** 子表字段配置转为提交的字段选项，去掉空项 */
function childFieldOptions(rows: ChildFieldModel[]): CodegenFieldOption[] {
  const out: CodegenFieldOption[] = [];
  for (const r of rows) {
    const column = r.column.trim();
    if (!column) continue;
    const o: CodegenFieldOption = { column };
    if (r.label.trim()) o.label = r.label.trim();
    if (r.image) o.upload = "image";
    if (r.hidden) o.form = false;
    if (r.hint.trim()) o.hint = r.hint.trim();
    if (r.search) o.search = "like";
    if (r.placeholder?.trim()) o.placeholder = r.placeholder.trim();
    if (Object.keys(o).length > 1) out.push(o);
  }
  return out;
}

export type AdvancedModel = {
  keyMode: CodegenKeyMode;
  readOnly: boolean;
  /** 关闭后 deleted_at 作为普通时间字段 */
  softDelete: boolean;
  refs: DeclaredRef[];
  children: ChildModel[];
  /** 列表和导出的排序，如 created_at DESC, id DESC */
  orderBy: string;
  /** 不生成新增、删除 */
  noCreate: boolean;
  noDelete: boolean;
  /** 删除确认文案，{列名} 换成该行的值 */
  deleteConfirm: string;
};

/** 提交给后端的子表配置：只保留所选键来源的项，去掉空值 */
export function childSpecs(children: ChildModel[]): CodegenChild[] {
  return children
    .filter(c => c.table)
    .map(({ keySource, fieldRows, requiredText, ...c }) => {
      const out: CodegenChild = {
        table: c.table.trim(),
        foreignKey: c.foreignKey.trim(),
        keyColumn: c.keyColumn.trim()
      };
      if (c.label?.trim()) out.label = c.label.trim();
      if (c.listKeys) {
        out.listKeys = true;
        if (c.listKeysLabel?.trim()) out.listKeysLabel = c.listKeysLabel.trim();
      }
      const fields = childFieldOptions(fieldRows ?? []);
      if (fields.length) out.fields = fields;
      const required = (requiredText ?? "")
        .split(/[,，]/)
        .map(v => v.trim())
        .filter(Boolean);
      if (required.length) out.requiredFields = required;
      if (c.hint?.trim()) out.hint = c.hint.trim();
      const columns = (c.columns ?? []).filter(
        col => col.key.trim() && col.field.trim() && col.label.trim()
      );
      if (columns.length) out.columns = columns;
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
  return children.map(({ fields, requiredFields, ...c }) => ({
    ...c,
    keySource: c.keyTable ? "table" : "dict",
    requiredText: (requiredFields ?? []).join(", "),
    fieldRows: (fields ?? []).map(f => ({
      column: f.column,
      label: f.label ?? "",
      image: f.upload === "image",
      hidden: f.form === false,
      hint: f.hint ?? "",
      search: f.search === "like",
      placeholder: f.placeholder ?? ""
    }))
  }));
}
