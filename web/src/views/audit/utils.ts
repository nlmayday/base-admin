import dayjs from "dayjs";
import { ref } from "vue";
import { listOperationLogModules } from "@/api/audit/operationLog";

export type TagType = "success" | "primary" | "warning" | "danger" | "info";

/** 时间范围选择器的值格式（RFC 3339，带时区） */
export const DATE_RANGE_FORMAT = "YYYY-MM-DD[T]HH:mm:ssZ";

/** 结束时间默认取当天 23:59:59 */
export const DATE_RANGE_DEFAULT_TIME: [Date, Date] = [
  new Date(2000, 0, 1, 0, 0, 0),
  new Date(2000, 0, 1, 23, 59, 59)
];

function lastDays(days: number): () => [Date, Date] {
  return () => [
    dayjs()
      .subtract(days - 1, "day")
      .startOf("day")
      .toDate(),
    dayjs().endOf("day").toDate()
  ];
}

export const DATE_RANGE_SHORTCUTS = [
  { text: "今天", value: lastDays(1) },
  { text: "近7天", value: lastDays(7) },
  { text: "近30天", value: lastDays(30) }
];

/** 时间范围 → startTime / endTime 查询参数 */
export function rangeParams(range: [string, string] | null | undefined): {
  startTime?: string;
  endTime?: string;
} {
  if (!range || range.length !== 2) return {};
  return { startTime: range[0], endTime: range[1] };
}

export const SUCCESS_OPTIONS: ReadonlyArray<{
  label: string;
  value: "true" | "false";
}> = [
  { label: "成功", value: "true" },
  { label: "失败", value: "false" }
];

/** 登录失败原因 */
export const REASON_LABELS: Record<string, string> = {
  bad_credentials: "用户名或密码错误",
  rate_limited: "请求过于频繁",
  captcha: "验证码错误",
  locked: "账号已锁定",
  error: "系统错误"
};

export function reasonLabel(reason: string): string {
  return REASON_LABELS[reason] ?? reason;
}

export const METHOD_TAG: Record<string, TagType> = {
  GET: "success",
  POST: "primary",
  PUT: "warning",
  PATCH: "warning",
  DELETE: "danger"
};

export function methodTag(method: string): TagType {
  return METHOD_TAG[method?.toUpperCase()] ?? "info";
}

export const METHOD_OPTIONS = [
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE"
] as const;

/** 模块中文名，取自各模块目录菜单的标题（含代码生成的模块）；未知模块显示模块标识 */
export const moduleNames = ref<{ value: string; label: string }[]>([]);

let moduleNamesLoaded: Promise<void> | null = null;

/** 加载模块名（只请求一次）；失败时显示模块标识 */
export function loadModuleNames(): Promise<void> {
  moduleNamesLoaded ??= listOperationLogModules()
    .then(list => {
      moduleNames.value = (list ?? []).map(m => ({
        value: m.name,
        label: m.title
      }));
    })
    .catch(() => {
      moduleNamesLoaded = null;
    });
  return moduleNamesLoaded;
}

export function moduleLabel(module: string): string {
  if (!module) return "-";
  return moduleNames.value.find(m => m.value === module)?.label ?? module;
}

/** 耗时着色：< 200ms 正常，< 1s 偏慢，≥ 1s 慢 */
export function durationClass(ms: number): string {
  if (ms >= 1000) return "text-[var(--el-color-danger)]";
  if (ms >= 200) return "text-[var(--el-color-warning)]";
  return "text-[var(--el-color-success)]";
}

/** HTTP 状态码着色 */
export function statusTag(status: number): TagType {
  if (status >= 500) return "danger";
  if (status >= 400) return "warning";
  return "success";
}

/** 能解析为 JSON 时格式化输出，否则原样返回 */
export function prettyJson(raw: string): string {
  if (!raw) return "";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

/** 按优先级匹配：Edge / Opera 的 UA 中也包含 Chrome 与 Safari */
const BROWSERS: ReadonlyArray<[RegExp, string]> = [
  [/Edg\/(\d+)/, "Edge"],
  [/OPR\/(\d+)/, "Opera"],
  [/Firefox\/(\d+)/, "Firefox"],
  [/Chrome\/(\d+)/, "Chrome"],
  [/Version\/(\d+)[\d.]* Safari\//, "Safari"],
  [/curl\/(\d+)/i, "curl"],
  [/PostmanRuntime\/(\d+)/, "Postman"]
];

const SYSTEMS =
  /(Windows NT [\d.]+|Mac OS X [\d_]+|Android [\d.]+|iPhone OS [\d_]+|Linux)/;

/** 从 User-Agent 中提取简短的浏览器 / 系统描述 */
export function shortUserAgent(ua: string): string {
  if (!ua) return "-";
  let name = "";
  for (const [re, label] of BROWSERS) {
    const m = re.exec(ua);
    if (m) {
      name = `${label} ${m[1]}`;
      break;
    }
  }
  const os = SYSTEMS.exec(ua);
  const system = os ? os[1].replace(/_/g, ".") : "";
  return [name, system].filter(Boolean).join(" / ") || ua;
}
