import { describe, expect, it } from "vitest";
import dayjs from "dayjs";
import { formatBytes, formatDateTime, formatUptime } from "./format";
import { offsetLabel, setDisplayZone } from "./timezone";

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
