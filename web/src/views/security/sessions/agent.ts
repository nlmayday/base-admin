/** 把 User-Agent 归纳成「浏览器 · 系统」，只用于展示 */
export function describeAgent(ua: string | null | undefined): string {
  if (!ua) return "未知终端";
  return `${browserOf(ua)} · ${systemOf(ua)}`;
}

const BROWSERS: readonly [RegExp, string][] = [
  [/Edg\/(\d+)/, "Edge"],
  [/OPR\/(\d+)/, "Opera"],
  [/Firefox\/(\d+)/, "Firefox"],
  [/Chrome\/(\d+)/, "Chrome"],
  [/Version\/(\d+).*Safari/, "Safari"]
];

function browserOf(ua: string): string {
  for (const [pattern, name] of BROWSERS) {
    const m = ua.match(pattern);
    if (m) return `${name} ${m[1]}`;
  }
  if (/curl|python|go-http|okhttp|postman/i.test(ua)) return "脚本/工具";
  return "其他浏览器";
}

function systemOf(ua: string): string {
  if (/iPhone|iPad|iOS/.test(ua)) return "iOS";
  if (/Android/.test(ua)) return "Android";
  if (/Windows/.test(ua)) return "Windows";
  if (/Mac OS X|Macintosh/.test(ua)) return "macOS";
  if (/Linux/.test(ua)) return "Linux";
  return "未知系统";
}

/** 「刚刚 / N 分钟前 / N 小时前 / N 天前」 */
export function relativeTime(
  value: string | null | undefined,
  now = Date.now()
): string {
  if (!value) return "-";
  const diff = Math.max(0, now - new Date(value).getTime());
  const minutes = Math.floor(diff / 60_000);
  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  return `${Math.floor(hours / 24)} 天前`;
}
