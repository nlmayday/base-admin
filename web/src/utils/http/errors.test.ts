import { describe, expect, it } from "vitest";
import {
  ApiError,
  ErrorCode,
  codeFromStatus,
  hasErrorCode,
  isApiError,
  needsReload
} from "./errors";

describe("ApiError", () => {
  it("keeps the envelope fields and defaults", () => {
    const err = new ApiError({ code: ErrorCode.NOT_FOUND, message: "不存在" });
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("不存在");
    expect(err.httpStatus).toBe(0);
    expect(err.data).toBeNull();
    expect(err.requestId).toBe("");
  });

  it("returns only well-formed field errors", () => {
    const err = new ApiError({
      code: ErrorCode.VALIDATION,
      message: "参数校验失败",
      data: {
        fields: [
          { field: "username", message: "不能为空" },
          { field: 1, message: "x" },
          null
        ]
      }
    });
    expect(err.fields).toEqual([{ field: "username", message: "不能为空" }]);
    expect(new ApiError({ code: 1, message: "", data: "x" }).fields).toEqual(
      []
    );
  });
});

describe("error helpers", () => {
  const conflict = new ApiError({ code: ErrorCode.CONFLICT, message: "" });

  it("recognises ApiError only", () => {
    expect(isApiError(conflict)).toBe(true);
    expect(isApiError(new Error("x"))).toBe(false);
    expect(hasErrorCode(conflict, ErrorCode.CONFLICT)).toBe(true);
    expect(hasErrorCode({ code: ErrorCode.CONFLICT }, ErrorCode.CONFLICT)).toBe(
      false
    );
  });

  it("asks to reload on version conflicts and unknown outcomes", () => {
    expect(needsReload(conflict)).toBe(true);
    expect(
      needsReload(
        new ApiError({ code: ErrorCode.OUTCOME_UNKNOWN, message: "" })
      )
    ).toBe(true);
    expect(
      needsReload(new ApiError({ code: ErrorCode.VALIDATION, message: "" }))
    ).toBe(false);
  });

  it.each([
    [401, ErrorCode.UNAUTHENTICATED],
    [403, ErrorCode.FORBIDDEN],
    [404, ErrorCode.NOT_FOUND],
    [409, ErrorCode.CONFLICT],
    [422, ErrorCode.VALIDATION],
    [429, ErrorCode.RATE_LIMITED],
    [503, ErrorCode.UNAVAILABLE],
    [500, ErrorCode.UNKNOWN]
  ])("maps HTTP %i to code %i", (status, code) => {
    expect(codeFromStatus(status)).toBe(code);
  });
});
