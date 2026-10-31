import { describe, expect, it } from "vitest";
import { FORCE_PASSWORD_PATH, LOGIN_PATH, safeRedirect } from "./paths";

describe("safeRedirect", () => {
  it("keeps in-site paths", () => {
    expect(safeRedirect("/system/user?page=2")).toBe("/system/user?page=2");
  });

  it.each([
    ["absolute URL", "https://evil.example/"],
    ["protocol-relative URL", "//evil.example/x"],
    ["relative path", "system/user"],
    ["login page", `${LOGIN_PATH}?redirect=/x`],
    ["password page", FORCE_PASSWORD_PATH],
    ["non-string", ["/a"]],
    ["empty", ""]
  ])("rejects %s (open redirect protection)", (_, value) => {
    expect(safeRedirect(value)).toBe("/");
    expect(safeRedirect(value, "/welcome")).toBe("/welcome");
  });
});
