export { formatBytes, formatUptime } from "@/utils/format";

/** 毫秒耗时转可读字符串 */
export function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return "-";
  if (ms < 1000) return `${ms} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 2 : 1)} s`;
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return `${minutes} 分 ${seconds} 秒`;
}

/** 秒数转「x 分钟」等超时描述 */
export function formatSeconds(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`;
  return `${seconds} 秒`;
}

/** 百分比颜色阈值：<60 正常，<85 警告，其余危险 */
export function percentColor(percent: number): string {
  if (percent < 60) return "#67c23a";
  if (percent < 85) return "#e6a23c";
  return "#f56c6c";
}
