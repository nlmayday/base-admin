import { http } from "@/utils/http";

/** 非开发环境调用代码生成接口时返回的错误码（403） */
export const CODEGEN_DISABLED = 80001;
/** 生成到项目时目标文件已存在（409） */
export const CODEGEN_EXISTS = 80002;

/** 字段的搜索方式 */
export type SearchMode = "none" | "like" | "eq" | "range";

export const SEARCH_MODE_LABELS: Record<SearchMode, string> = {
  none: "不搜索",
  like: "模糊",
  eq: "精确",
  range: "范围"
};

/** 数据表字段及其默认配置 */
export type CodegenFieldInfo = {
  column: string;
  /** 类型的中文说明，如 文本、金额/定点数 */
  type: string;
  comment: string;
  label: string;
  list: boolean;
  form: boolean;
  /** 表单开关不可改（系统字段、必填无默认值等），formHint 说明原因 */
  formLocked: boolean;
  formHint?: string;
  search: SearchMode;
  searchModes: SearchMode[];
  /** 可以使用下拉选项（可编辑的文本、整数字段） */
  dictAllowed: boolean;
  /** 下拉选项来源：字典编码、OPTIONS_FROM_COMMENT（注释中的选项）或空（不使用） */
  dict: string;
  /** 从注释「类型：1=通知,2=公告」解析出的选项 */
  commentOptions: { value: string; label: string }[];
  /** 上传控件：image 图片（存地址），files 附件列表（JSON 字段），空为不使用 */
  upload: UploadMode;
  /** 该字段可用的上传控件 */
  uploadModes: Exclude<UploadMode, "">[];
  /** 关联字段（外键）显示的列，空为普通 ID 输入 */
  relation: string;
  /** 外键指向的表 */
  relationTable: string;
  /** 可作为显示列的文本列 */
  relationLabels: string[];
};

export type UploadMode = "" | "image" | "files";

/** 下拉选项来源：使用字段注释中的选项 */
export const OPTIONS_FROM_COMMENT = "@comment";

/** 可供绑定的数据字典 */
export type CodegenDict = { code: string; name: string; enabled: boolean };

/** 提交给后端的字段配置，只包含与默认值不同的项 */
export type CodegenFieldOption = {
  column: string;
  label?: string;
  list?: boolean;
  form?: boolean;
  search?: SearchMode;
  dict?: string;
  upload?: UploadMode;
  relation?: string;
};

export type CodegenTable = {
  name: string;
  comment: string;
  /** 不满足生成要求时为 false，reason 说明原因 */
  supported: boolean;
  reason: string;
  columns: number;
};

export type CodegenRequest = {
  table: string;
  module: string;
  resource: string;
  name: string;
  moduleTitle: string;
  fields?: CodegenFieldOption[];
};

export type CodegenGenerateResult = {
  /** 写入的文件（相对仓库根目录） */
  files: string[];
  /** 本次新建了模块（否则是在已有模块中新增资源） */
  newModule: boolean;
  /** 已自动接入：新模块写入 modules.go，新资源写入所属模块的 module.go */
  wired: boolean;
  notes: string[];
};

export type CodegenLanguage = "go" | "typescript" | "vue";

export type CodegenFile = {
  /** 相对仓库根目录的路径 */
  path: string;
  language: CodegenLanguage;
  content: string;
  /** 文件已存在：CLI 不加 -force 会拒绝覆盖 */
  exists: boolean;
};

export type CodegenMenu = {
  code: string;
  type: string;
  title: string;
  path: string;
  component: string;
};

export type CodegenPreview = {
  files: CodegenFile[];
  newModule: boolean;
  permissions: { code: string; name: string }[];
  menus: CodegenMenu[];
  /** 需要加到 server/internal/app/modules.go 的代码 */
  wiring: string;
  notes: string[];
  /** 等价的 CLI 命令 */
  command: string;
};

/** 错误由页面自行处理（非开发环境时显示说明而不是弹出提示） */
export const listCodegenTables = () =>
  http.get<CodegenTable[]>("/devtools/codegen/tables", undefined, {
    skipErrorHandler: true
  });

export const previewCodegen = (data: CodegenRequest) =>
  http.post<CodegenPreview>("/devtools/codegen/preview", data);

export const getCodegenFields = (table: string) =>
  http.get<CodegenFieldInfo[]>("/devtools/codegen/fields", { table });

/** 已存在文件时由页面提示，不走统一错误弹窗 */
export const generateCodegen = (data: CodegenRequest & { force: boolean }) =>
  http.post<CodegenGenerateResult>("/devtools/codegen/generate", data, {
    skipErrorHandler: true
  });

export type CodegenLastGeneration = {
  request: CodegenRequest & { force: boolean };
  result: CodegenGenerateResult;
  at: string;
};

/** 当前用户最近一次「生成到项目」的结果；开发服务器刷新页面后用于恢复 */
export const getLastCodegen = () =>
  http.get<CodegenLastGeneration | null>("/devtools/codegen/last", undefined, {
    skipErrorHandler: true
  });

export const listCodegenDicts = () =>
  http.get<CodegenDict[]>("/devtools/codegen/dicts", undefined, {
    skipErrorHandler: true
  });
