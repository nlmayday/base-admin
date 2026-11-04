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
  /** 在 CSV 导出中（默认为主键和列表显示的列） */
  export: boolean;
  form: boolean;
  /** 表单开关不可改（系统字段、必填无默认值等），formHint 说明原因 */
  formLocked: boolean;
  formHint?: string;
  search: SearchMode;
  searchModes: SearchMode[];
  /** 可以使用下拉选项（文本、整数、是/否字段；是/否字段须不在表单中） */
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
  /** 是否导出；不传时跟随列表显示 */
  export?: boolean;
  form?: boolean;
  search?: SearchMode;
  dict?: string;
  upload?: UploadMode;
  relation?: string;
  /** 不依赖外键声明的关联：本列（bigint）指向 refTable.refKey（默认 id），relation 为显示列 */
  refTable?: string;
  refKey?: string;
};

/** 主键来源：auto 数据库自增，generated 程序生成（雪花 ID），input 新增时填写；空为按列自动判断 */
export type CodegenKeyMode = "" | "auto" | "generated" | "input";

/** 随主表一起编辑的子表（如多语言表），每个键（如语言）一行 */
export type CodegenChild = {
  table: string;
  /** 子表中指向主表主键的列 */
  foreignKey: string;
  /** 区分同一主表行下各子表行的列，如 lang */
  keyColumn: string;
  /** 列出键值的数据字典，如 lang（与 keyTable 二选一） */
  keyDict?: string;
  /** 列出键值的表（如与游戏服共用的 lg_language），keyValueColumn 为值，keyLabelColumn 为名称 */
  keyTable?: string;
  keyValueColumn?: string;
  keyLabelColumn?: string;
  /** 键表的过滤条件，如 enabled = 1 */
  keyFilter?: string;
  keySortColumn?: string;
  /** 键表中表示「必须填写」的列，如 required */
  keyRequiredColumn?: string;
  /** 表单中的分组标题，如 多语言 */
  label?: string;
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
  keyMode?: CodegenKeyMode;
  /** 只读：只生成列表、详情和导出 */
  readOnly?: boolean;
  /** false：deleted_at 作为普通时间字段（如注销时间），列出全部行，删除即删除行 */
  softDelete?: boolean;
  children?: CodegenChild[];
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
  /** 扩展文件（自定义代码写在这里）：只在不存在时写入，永不覆盖 */
  keep: boolean;
  /** 扩展文件已存在，生成时保留原样 */
  kept: boolean;
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
