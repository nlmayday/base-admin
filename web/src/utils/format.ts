import dayjs from "dayjs";
import { inDisplayZone, zoneLabel } from "./timezone";

/**
 * RFC 3339 时间按顶栏选择的时区（浏览器时区或 UTC）显示，并标注时区，
 * 如 2026-11-04 10:00:00 (GMT+8)；空值显示 "-"。
 */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "-";
  if (!dayjs(value).isValid()) return value;
  return `${inDisplayZone(value).format("YYYY-MM-DD HH:mm:ss")} (${zoneLabel(value)})`;
}

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

/** 字节数转可读字符串，如 1.5 GB；无效值显示 "-" */
export function formatBytes(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "-";
  }
  if (value < 0) return "-";
  let v = value;
  let unit = 0;
  while (v >= 1024 && unit < BYTE_UNITS.length - 1) {
    v /= 1024;
    unit++;
  }
  const digits = unit === 0 || v >= 100 ? 0 : 1;
  return `${v.toFixed(digits).replace(/\.0$/, "")} ${BYTE_UNITS[unit]}`;
}

/** 秒数转「x 天 y 小时 z 分」 */
export function formatUptime(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds < 0) return "-";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时 ${minutes} 分`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分`;
  if (minutes > 0) return `${minutes} 分 ${Math.floor(seconds % 60)} 秒`;
  return `${Math.floor(seconds)} 秒`;
}

/** 数字列的显示格式（代码生成的 format），导出仍为原值 */
type Num = number | string | null | undefined;

function toNumber(v: Num): number | null {
  if (v === null || v === undefined || v === "") return null;
  const n = typeof v === "number" ? v : Number(v);
  return Number.isFinite(n) ? n : null;
}

function grouped(n: number, digits?: number): string {
  return n.toLocaleString("en-US", {
    minimumFractionDigits: digits ?? 0,
    maximumFractionDigits: digits ?? 20
  });
}

/** 千分位：1234567 → 1,234,567；空值 "-" */
export function formatCoins(v: Num): string {
  const n = toNumber(v);
  return n === null ? "-" : grouped(n);
}

/** 千分位带符号：+1,234 / -56 / 0；颜色用 signClass */
export function formatSigned(v: Num): string {
  const n = toNumber(v);
  if (n === null) return "-";
  return n > 0 ? `+${grouped(n)}` : grouped(n);
}

/** 正数绿、负数红（配合 formatSigned） */
export function signClass(v: Num): string {
  const n = toNumber(v);
  if (!n) return "";
  return n > 0
    ? "text-[var(--el-color-success)]"
    : "text-[var(--el-color-danger)]";
}

/** 美分显示成美元：12345 → $123.45，-5 → -$0.05 */
export function formatCents(v: Num): string {
  const n = toNumber(v);
  if (n === null) return "-";
  const s = `$${grouped(Math.abs(n) / 100, 2)}`;
  return n < 0 ? `-${s}` : s;
}

/** 千分比显示成百分比：55 → +5.5%，-10 → -1%，0 → 0% */
export function formatPermille(v: Num): string {
  const n = toNumber(v);
  if (n === null) return "-";
  const pct = `${Number((n / 10).toFixed(2))}%`;
  return n > 0 ? `+${pct}` : pct;
}

/** 千分之一的值：1500 → 1.50，加后缀如 "x" → 1.50x */
export function formatMilli(v: Num, suffix = ""): string {
  const n = toNumber(v);
  return n === null ? "-" : `${grouped(n / 1000, 2)}${suffix}`;
}

/** 关联字段的显示：名称后跟附加列，如 100001（玩家甲）；都为空时 "-" */
export function relationLabel(
  label: string | null | undefined,
  extra?: string | null
): string {
  if (!label) return extra || "-";
  return extra ? `${label}（${extra}）` : label;
}
