/**
 * Cron 表达式的中文描述（与后端 robfig/cron v3 标准 5 段语法一致：
 * 分 时 日 月 周，另支持 @daily / @every 5m 等描述符）。
 * 只负责展示；合法性以后端 POST /jobs/cron/preview 为准。
 */

export type CronPreset = { label: string; expr: string };

export const CRON_PRESETS: CronPreset[] = [
  { label: "每分钟", expr: "* * * * *" },
  { label: "每 5 分钟", expr: "*/5 * * * *" },
  { label: "每小时", expr: "0 * * * *" },
  { label: "每天凌晨", expr: "0 0 * * *" },
  { label: "每周一", expr: "0 0 * * 1" }
];

const DESCRIPTORS: Record<string, string> = {
  "@yearly": "每年 1 月 1 日 00:00",
  "@annually": "每年 1 月 1 日 00:00",
  "@monthly": "每月 1 日 00:00",
  "@weekly": "每周日 00:00",
  "@daily": "每天 00:00",
  "@midnight": "每天 00:00",
  "@hourly": "每小时整点"
};

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六", "日"];
const DOW_NAMES = ["SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"];
const MONTH_NAMES = [
  "JAN",
  "FEB",
  "MAR",
  "APR",
  "MAY",
  "JUN",
  "JUL",
  "AUG",
  "SEP",
  "OCT",
  "NOV",
  "DEC"
];

const DURATION_UNITS: Record<string, string> = {
  h: "小时",
  m: "分钟",
  s: "秒"
};

function describeEvery(value: string): string {
  const parts = value.match(/(\d+(?:\.\d+)?)(h|m|s)/g);
  if (!parts || parts.join("") !== value) return `每隔 ${value}`;
  const text = parts
    .map(p => {
      const [, n, unit] = /(\d+(?:\.\d+)?)(h|m|s)/.exec(p)!;
      return `${n} ${DURATION_UNITS[unit]}`;
    })
    .join(" ");
  return `每隔 ${text}`;
}

const pad = (v: string) => (/^\d$/.test(v) ? `0${v}` : v);

/** 把月份、星期名称换成数字 */
function normalizeNames(field: string, names: string[], offset: number) {
  let out = field.toUpperCase();
  names.forEach((name, i) => {
    out = out.replaceAll(name, String(i + offset));
  });
  return out;
}

/** 单个字段转中文，如 1-5 → 1 至 5，1,3 → 1、3 */
function describeField(field: string, map?: (v: string) => string): string {
  const fmt = map ?? ((v: string) => v);
  return field
    .split(",")
    .map(part => {
      const [range, step] = part.split("/");
      let text = range === "*" ? "" : range.split("-").map(fmt).join(" 至 ");
      if (step) text = `${text ? text + " 内" : ""}每 ${step}`;
      return text;
    })
    .join("、");
}

const isNumber = (f: string) => /^\d+$/.test(f);

function describeTime(minute: string, hour: string): string {
  if (isNumber(minute) && isNumber(hour)) return `${pad(hour)}:${pad(minute)}`;
  if (minute === "0") {
    if (hour === "*") return "每小时整点";
    if (/^\*\/\d+$/.test(hour)) return `每 ${hour.slice(2)} 小时整点`;
    return `${describeField(hour)} 点整`;
  }
  let minutePart: string;
  if (minute === "*") minutePart = "每分钟";
  else if (/^\*\/\d+$/.test(minute)) minutePart = `每 ${minute.slice(2)} 分钟`;
  else minutePart = `第 ${describeField(minute)} 分`;

  if (hour === "*") {
    return minute === "*" || minute.startsWith("*/")
      ? minutePart
      : `每小时${minutePart}`;
  }
  if (/^\*\/\d+$/.test(hour)) {
    return `每 ${hour.slice(2)} 小时的${minutePart}`;
  }
  return `${describeField(hour)} 点的${minutePart}`;
}

function describeDays(dom: string, month: string, dow: string): string {
  const parts: string[] = [];
  if (month !== "*") parts.push(`每年 ${describeField(month)} 月`);
  if (dom !== "*") {
    parts.push(`${parts.length ? "" : "每月 "}${describeField(dom)} 日`);
  }
  if (dow !== "*") {
    const days = describeField(dow, v => `周${WEEKDAYS[Number(v)] ?? v}`);
    parts.push(`${dom !== "*" ? "或" : ""}每${days}`);
  }
  if (parts.length === 0) return "每天";
  if (dom === "*" && dow === "*") parts.push("每天");
  return parts.join(" ");
}

/** 返回 Cron 表达式的中文描述；无法识别时返回空字符串 */
export function describeCron(expr: string): string {
  const value = (expr ?? "").trim();
  if (!value) return "";
  const lower = value.toLowerCase();
  if (DESCRIPTORS[lower]) return DESCRIPTORS[lower];
  if (lower.startsWith("@every ")) return describeEvery(lower.slice(7).trim());
  const fields = value.split(/\s+/);
  if (fields.length !== 5) return "";
  if (fields.some(f => !/^[\d*,/\-A-Za-z]+$/.test(f))) return "";
  const [minute, hour, dom, rawMonth, rawDow] = fields;
  const month = normalizeNames(rawMonth, MONTH_NAMES, 1);
  const dow = normalizeNames(rawDow, DOW_NAMES, 0);
  const time = describeTime(minute, hour);
  const days = describeDays(dom, month, dow);
  if (days === "每天" && (hour === "*" || hour.startsWith("*/"))) return time;
  return `${days} ${time}`;
}
