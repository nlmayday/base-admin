/** 按小时给出问候语 */
export function greetingOf(hour: number): string {
  if (hour < 6) return "夜深了";
  if (hour < 9) return "早上好";
  if (hour < 12) return "上午好";
  if (hour < 14) return "中午好";
  if (hour < 18) return "下午好";
  return "晚上好";
}

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

/** 「2026年10月30日 星期五」 */
export function todayLabel(d: Date): string {
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 星期${WEEKDAYS[d.getDay()]}`;
}
