import { describe, expect, it } from "vitest";
import { ApiError, ErrorCode } from "@/utils/http/errors";
import { useFieldErrors } from "./useFieldErrors";

describe("useFieldErrors", () => {
  it("shows validation errors on their fields", () => {
    const f = useFieldErrors();
    const err = new ApiError({
      code: ErrorCode.VALIDATION,
      message: "参数校验失败",
      data: { fields: [{ field: "username", message: "用户名已存在" }] }
    });
    expect(f.applyFieldErrors(err)).toBe(true);
    expect(f.fieldErrors.value).toEqual({ username: "用户名已存在" });
    f.clearFieldErrors();
    expect(f.fieldErrors.value).toEqual({});
  });

  it("leaves other errors to the caller", () => {
    const f = useFieldErrors();
    expect(f.applyFieldErrors(new Error("x"))).toBe(false);
    expect(
      f.applyFieldErrors(
        new ApiError({ code: ErrorCode.CONFLICT, message: "" })
      )
    ).toBe(false);
    expect(
      f.applyFieldErrors(
        new ApiError({ code: ErrorCode.VALIDATION, message: "", data: {} })
      )
    ).toBe(false);
    expect(f.fieldErrors.value).toEqual({});
  });
});
