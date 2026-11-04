import { afterEach, describe, expect, it } from "vitest";
import {
  FALLBACK_PAGE_SIZE,
  defaultPageSize,
  setDefaultPageSize
} from "./pageSize";

describe("defaultPageSize", () => {
  afterEach(() => setDefaultPageSize(FALLBACK_PAGE_SIZE));

  it("uses the setting when it is one of the page sizes", () => {
    expect(defaultPageSize()).toBe(20);
    setDefaultPageSize(50);
    expect(defaultPageSize()).toBe(50);
    setDefaultPageSize(30);
    expect(defaultPageSize()).toBe(50);
    setDefaultPageSize(undefined);
    expect(defaultPageSize()).toBe(50);
  });
});
