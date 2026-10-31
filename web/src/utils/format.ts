import dayjs from "dayjs";

/** RFC 3339 时间转本地显示；空值显示 "-" */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "-";
  const d = dayjs(value);
  return d.isValid() ? d.format("YYYY-MM-DD HH:mm:ss") : value;
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
