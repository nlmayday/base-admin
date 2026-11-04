import { describe, expect, it } from "vitest";
import { offsetLabel, toUTC, utcHint } from "./timezone";

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
});
