import type {
  CodegenFieldExtras,
  CodegenFieldInfo
} from "@/api/devtools/codegen";

/** 「条件显示」的取值在页面上写成逗号分隔的文本 */
export function splitValues(text: string): string[] {
  return text
    .split(/[,，]/)
    .map(v => v.trim())
    .filter(Boolean);
}

/** 字段的显示、表单、搜索细节中设置了的项（提交给后端） */
export function fieldExtras(f: CodegenFieldInfo): CodegenFieldExtras {
  const o: CodegenFieldExtras = {};
  if (f.format) o.format = f.format;
  if (f.format === "milli" && f.suffix?.trim()) o.suffix = f.suffix.trim();
  if (f.relation && f.relationExtra) o.relationExtra = f.relationExtra;
  if (f.searchDefault?.trim() && f.search !== "none" && f.search !== "range") {
    o.searchDefault = f.searchDefault.trim();
  }
  if (f.form) {
    if (f.default?.trim()) o.default = f.default.trim();
    if (f.placeholder?.trim()) o.placeholder = f.placeholder.trim();
    for (const k of ["min", "max", "step"] as const) {
      const v = f[k];
      if (typeof v === "number" && Number.isFinite(v)) o[k] = v;
    }
    if (f.showWhen?.column && f.showWhen.in.length) {
      o.showWhen = { column: f.showWhen.column, in: [...f.showWhen.in] };
    }
    if (f.required === false && f.textRequired) o.required = false;
  }
  const feature = f.showIf?.trim().replace(/^feature:/, "");
  if (feature) o.showIf = `feature:${feature}`;
  return o;
}
