import dayjs, { type Dayjs } from "dayjs";
import utc from "dayjs/plugin/utc";
import { ref } from "vue";

dayjs.extend(utc);

/**
 * 时间的显示时区：local 为浏览器时区，utc 为 UTC。顶栏切换，记在浏览器里。
 * 接口一律收发 UTC 的 RFC 3339 时间，这里只影响显示。
 */
export type DisplayZone = "local" | "utc";

const STORAGE_KEY = "display-timezone";

function loadZone(): DisplayZone {
  try {
    return localStorage.getItem(STORAGE_KEY) === "utc" ? "utc" : "local";
  } catch {
    return "local";
  }
}

/** 当前显示时区（响应式，模板里用到的时间会随切换刷新） */
export const displayZone = ref<DisplayZone>(loadZone());

export function setDisplayZone(zone: DisplayZone): void {
  displayZone.value = zone;
  try {
    localStorage.setItem(STORAGE_KEY, zone);
  } catch {
    // 无痕模式等无法存储时，只在本次页面内生效
  }
}

/** UTC 偏移（分钟）的标注，如 480 → GMT+8，330 → GMT+5:30，0 → GMT+0 */
export function offsetLabel(minutes: number): string {
  const sign = minutes < 0 ? "-" : "+";
  const abs = Math.abs(minutes);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  return `GMT${sign}${h}${m ? `:${String(m).padStart(2, "0")}` : ""}`;
}

/** 浏览器时区在 at（默认现在）的标注，如 GMT+8；夏令时按当天计算 */
export function localZoneLabel(at?: string | Date): string {
  return offsetLabel(dayjs(at).utcOffset());
}

/** 当前显示时区的标注：UTC 或 GMT+8 */
export function zoneLabel(at?: string | Date): string {
  return displayZone.value === "utc" ? "UTC" : localZoneLabel(at);
}

/** 把时间换到当前显示时区 */
export function inDisplayZone(value: string | Date): Dayjs {
  return displayZone.value === "utc" ? dayjs(value).utc() : dayjs(value);
}

/** 时间转 UTC 的 RFC 3339 字符串（发给接口），空值原样返回 */
export function toUTC<T extends string | Date | null | undefined>(
  value: T
): string | Exclude<T, Date> {
  if (!value) return value as Exclude<T, Date>;
  const d = dayjs(value);
  return d.isValid()
    ? d.utc().format("YYYY-MM-DD[T]HH:mm:ss[Z]")
    : (value as Exclude<T, Date>);
}

/**
 * 时间在另一个时区的写法（显示为本地时给 UTC，显示为 UTC 时给本地），
 * 用于悬停提示；浏览器本身就在 UTC 或值无效时为 ""。
 */
export function otherZoneText(value: string | Date | null | undefined): string {
  if (!value || localZoneLabel(value) === "GMT+0") return "";
  const d = dayjs(value);
  if (!d.isValid()) return "";
  return displayZone.value === "utc"
    ? `${d.format("YYYY-MM-DD HH:mm:ss")} (${localZoneLabel(value)})`
    : `${d.utc().format("YYYY-MM-DD HH:mm:ss")} (UTC)`;
}

/** 按顶栏时区读写的墙上时间 "YYYY-MM-DD HH:mm:ss"，与 RFC 3339 互转 */
export function toZoneWall(value: string | null | undefined): string {
  if (!value) return "";
  const d = dayjs(value);
  return d.isValid()
    ? inDisplayZone(d.toDate()).format("YYYY-MM-DD HH:mm:ss")
    : "";
}

/** 顶栏时区的墙上时间转 UTC 的 RFC 3339；空值为 "" */
export function fromZoneWall(wall: string | null | undefined): string {
  if (!wall) return "";
  const d = displayZone.value === "utc" ? dayjs.utc(wall) : dayjs(wall);
  return d.isValid() ? d.utc().format("YYYY-MM-DD[T]HH:mm:ss[Z]") : "";
}

/** 选择器里的本地时间对应的 UTC，如 2026-11-04 02:00:00 UTC；空值为 "" */
export function utcHint(value: string | Date | null | undefined): string {
  if (!value) return "";
  const d = dayjs(value);
  return d.isValid() ? `${d.utc().format("YYYY-MM-DD HH:mm:ss")} UTC` : "";
}
