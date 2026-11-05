import { describe, expect, it } from "vitest";
import type { CodegenFieldInfo } from "@/api/devtools/codegen";
import { fieldExtras, splitValues } from "./extras";

const base = {
  column: "reward",
  form: true,
  search: "eq",
  relation: "",
  textRequired: false
} as CodegenFieldInfo;

describe("fieldExtras", () => {
  it("keeps the options that are set", () => {
    expect(
      fieldExtras({
        ...base,
        format: "milli",
        suffix: " x ",
        searchDefault: " 0 ",
        default: "100",
        min: 0,
        max: undefined,
        step: 10,
        placeholder: " 金币 ",
        showWhen: { column: "fill_type", in: ["2"] },
        showIf: "vip"
      })
    ).toEqual({
      format: "milli",
      suffix: "x",
      searchDefault: "0",
      default: "100",
      min: 0,
      step: 10,
      placeholder: "金币",
      showWhen: { column: "fill_type", in: ["2"] },
      showIf: "feature:vip"
    });
  });

  it("drops what does not apply", () => {
    expect(
      fieldExtras({
        ...base,
        form: false,
        search: "range",
        format: "coins",
        suffix: "x",
        searchDefault: "1",
        default: "1",
        required: false,
        relationExtra: "nickname"
      })
    ).toEqual({ format: "coins" });
    expect(
      fieldExtras({ ...base, textRequired: true, required: false })
    ).toEqual({ required: false });
  });

  it("splits the showWhen values", () => {
    expect(splitValues("1, 2，3 ,")).toEqual(["1", "2", "3"]);
  });
});
