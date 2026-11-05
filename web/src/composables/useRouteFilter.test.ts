import { describe, expect, it } from "vitest";
import { applyRouteQuery } from "./useRouteFilter";

describe("applyRouteQuery", () => {
  it("fills the filters named in the query", () => {
    const filter: Record<string, unknown> = {
      userId: null,
      amountMin: undefined,
      isTest: null,
      orderNo: ""
    };
    const ranges: Record<string, string[] | null> = { createdAt: null };
    const hit = applyRouteQuery(
      {
        userId: "1001",
        amountMin: "5",
        isTest: "1",
        other: "x",
        createdAtStart: "2026-01-01"
      },
      {
        filter,
        kinds: {
          userId: "string",
          amountMin: "number",
          isTest: "boolean",
          orderNo: "string"
        },
        ranges
      }
    );
    expect(hit).toBe(true);
    expect(filter).toEqual({
      userId: "1001",
      amountMin: 5,
      isTest: true,
      orderNo: ""
    });
    expect(ranges.createdAt).toEqual(["2026-01-01", "2026-01-01"]);
  });

  it("ignores bad numbers and an empty query", () => {
    const filter: Record<string, unknown> = { level: undefined };
    expect(
      applyRouteQuery({ level: "abc" }, { filter, kinds: { level: "number" } })
    ).toBe(false);
    expect(filter.level).toBeUndefined();
    expect(applyRouteQuery({}, { filter, kinds: { level: "number" } })).toBe(
      false
    );
  });
});
