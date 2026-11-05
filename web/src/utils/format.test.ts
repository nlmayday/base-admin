import { describe, expect, it } from "vitest";
import dayjs from "dayjs";
import {
  formatBytes,
  formatCents,
  formatCoins,
  formatDateTime,
  formatMilli,
  formatPermille,
  formatSigned,
  formatUptime,
  relationLabel,
  signClass
} from "./format";
import { offsetLabel, setDisplayZone } from "./timezone";

describe("list formats", () => {
  it("formats numbers for display", () => {
    expect(formatCoins(1234567)).toBe("1,234,567");
    expect(formatCoins("1234.5")).toBe("1,234.5");
    expect(formatCoins(null)).toBe("-");
    expect(formatSigned(1234)).toBe("+1,234");
    expect(formatSigned(-56)).toBe("-56");
    expect(formatSigned(0)).toBe("0");
    expect(signClass(3)).toContain("success");
    expect(signClass(-3)).toContain("danger");
    expect(signClass(0)).toBe("");
    expect(formatCents(12345)).toBe("$123.45");
    expect(formatCents(-5)).toBe("-$0.05");
    expect(formatCents(123456789)).toBe("$1,234,567.89");
    expect(formatPermille(55)).toBe("+5.5%");
    expect(formatPermille(-10)).toBe("-1%");
    expect(formatPermille(0)).toBe("0%");
    expect(formatMilli(1500, "x")).toBe("1.50x");
    expect(formatMilli(2000000)).toBe("2,000.00");
    expect(relationLabel("100001", "玩家甲")).toBe("100001（玩家甲）");
    expect(relationLabel("100001", "")).toBe("100001");
    expect(relationLabel("", "")).toBe("-");
  });
});

describe("formatBytes", () => {
  it.each([
    [0, "0 B"],
    [1023, "1023 B"],
    [1536, "1.5 KB"],
    [10 * 1024 * 1024, "10 MB"],
    [150 * 1024 * 1024, "150 MB"],
    [1.5 * 1024 ** 3, "1.5 GB"]
  ])("%d → %s", (value, text) => {
    expect(formatBytes(value)).toBe(text);
  });

  it.each([null, undefined, -1, Number.NaN, Infinity])("%s → -", value => {
    expect(formatBytes(value)).toBe("-");
  });
});

describe("formatUptime", () => {
  it.each([
    [42, "42 秒"],
    [125, "2 分 5 秒"],
    [3 * 3600 + 120, "3 小时 2 分"],
    [2 * 86400 + 3600, "2 天 1 小时 0 分"],
    [-1, "-"]
  ])("%d → %s", (seconds, text) => {
    expect(formatUptime(seconds)).toBe(text);
  });
});

describe("formatDateTime", () => {
  it("shows - for empty and the raw text for invalid values", () => {
    expect(formatDateTime(null)).toBe("-");
    expect(formatDateTime("")).toBe("-");
    expect(formatDateTime("not a date")).toBe("not a date");
  });

  it("formats RFC 3339 times in the browser time zone with its offset", () => {
    setDisplayZone("local");
    const at = "2026-10-30T08:09:10Z";
    const local = dayjs(at);
    expect(formatDateTime(at)).toBe(
      `${local.format("YYYY-MM-DD HH:mm:ss")} (${offsetLabel(local.utcOffset())})`
    );
  });

  it("formats in UTC when UTC is chosen", () => {
    setDisplayZone("utc");
    expect(formatDateTime("2026-10-30T16:09:10+08:00")).toBe(
      "2026-10-30 08:09:10 (UTC)"
    );
    setDisplayZone("local");
  });
});
