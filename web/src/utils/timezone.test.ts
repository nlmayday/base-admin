import { describe, expect, it } from "vitest";
import dayjs from "dayjs";
import {
  fromZoneWall,
  offsetLabel,
  otherZoneText,
  setDisplayZone,
  toUTC,
  toZoneWall,
  utcHint
} from "./timezone";

describe("timezone", () => {
  it("labels offsets", () => {
    expect(offsetLabel(480)).toBe("GMT+8");
    expect(offsetLabel(330)).toBe("GMT+5:30");
    expect(offsetLabel(-300)).toBe("GMT-5");
    expect(offsetLabel(0)).toBe("GMT+0");
  });

  it("sends UTC RFC 3339 times", () => {
    expect(toUTC("2026-11-04T10:00:00+08:00")).toBe("2026-11-04T02:00:00Z");
    expect(toUTC("")).toBe("");
    expect(toUTC(null)).toBe(null);
    expect(utcHint("2026-11-04T10:00:00+08:00")).toBe(
      "2026-11-04 02:00:00 UTC"
    );
    expect(utcHint(null)).toBe("");
  });

  it("reads and writes picker times in the header's zone", () => {
    setDisplayZone("utc");
    expect(toZoneWall("2026-11-04T02:00:00Z")).toBe("2026-11-04 02:00:00");
    expect(fromZoneWall("2026-11-04 02:00:00")).toBe("2026-11-04T02:00:00Z");
    expect(fromZoneWall("")).toBe("");
    setDisplayZone("local");
    const wall = toZoneWall("2026-11-04T02:00:00Z");
    expect(wall).toBe(
      dayjs("2026-11-04T02:00:00Z").format("YYYY-MM-DD HH:mm:ss")
    );
    expect(fromZoneWall(wall)).toBe("2026-11-04T02:00:00Z");
    const other = otherZoneText("2026-11-04T02:00:00Z");
    if (dayjs().utcOffset() !== 0) {
      expect(other).toBe("2026-11-04 02:00:00 (UTC)");
    } else {
      expect(other).toBe("");
    }
  });
});
