import { describe, expect, it, vi } from "vitest";
import { hasPerm } from "./permission";

vi.mock("@/store/modules/user", () => ({
  useUserStoreHook: () => ({
    permissions: new Set(["iam:user:view", "iam:user:create"])
  })
}));

describe("hasPerm", () => {
  it("checks one code", () => {
    expect(hasPerm("iam:user:view")).toBe(true);
    expect(hasPerm("iam:user:delete")).toBe(false);
  });

  it("requires every code of a list", () => {
    expect(hasPerm(["iam:user:view", "iam:user:create"])).toBe(true);
    expect(hasPerm(["iam:user:view", "iam:user:delete"])).toBe(false);
  });

  it("denies an empty requirement", () => {
    expect(hasPerm([])).toBe(false);
    expect(hasPerm("")).toBe(false);
  });
});
