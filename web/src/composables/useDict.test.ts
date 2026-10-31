import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";
import { getDictOptions } from "@/api/system/dict";
import { invalidateDict, staticDict, useDict } from "./useDict";

vi.mock("@/api/system/dict", () => ({ getDictOptions: vi.fn() }));
const fetchDict = vi.mocked(getDictOptions);

const flush = () => new Promise(resolve => setTimeout(resolve));

beforeEach(() => {
  invalidateDict();
  fetchDict.mockReset();
});

describe("useDict", () => {
  it("loads items once for concurrent users and serves labels and tags", async () => {
    fetchDict.mockResolvedValue([
      { value: "1", label: "启用", tagType: "success" },
      { value: "2", label: "停用", tagType: "" }
    ]);
    const a = useDict("status");
    const b = useDict("status");
    await flush();
    expect(fetchDict).toHaveBeenCalledTimes(1);
    expect(a.items.value).toHaveLength(2);
    expect(b.labelOf(1)).toBe("启用");
    expect(a.labelOf("9")).toBe("9");
    expect(a.labelOf(null)).toBe("-");
    expect(a.tagTypeOf("1")).toBe("success");
    expect(a.tagTypeOf("2")).toBeUndefined();
  });

  it("reloads after invalidation and follows a reactive code", async () => {
    fetchDict.mockImplementation(async code => [
      { value: code, label: code.toUpperCase(), tagType: "" }
    ]);
    const code = ref("a");
    const dict = useDict(code);
    await flush();
    expect(dict.labelOf("a")).toBe("A");
    code.value = "b";
    await nextTick();
    await flush();
    expect(dict.labelOf("b")).toBe("B");
    await dict.reload();
    expect(fetchDict).toHaveBeenCalledTimes(3);
  });

  it("falls back to no items when the request fails", async () => {
    fetchDict.mockRejectedValue(new Error("network"));
    const dict = useDict("broken");
    await flush();
    expect(dict.items.value).toEqual([]);
    expect(dict.loading.value).toBe(false);
  });
});

describe("staticDict", () => {
  it("has the same shape as useDict for fixed options", () => {
    const dict = staticDict([
      { value: "1", label: "通知" },
      { value: "2", label: "公告" }
    ]);
    expect(dict.items.value.map(i => i.label)).toEqual(["通知", "公告"]);
    expect(dict.labelOf(2)).toBe("公告");
    expect(dict.labelOf("")).toBe("-");
    expect(dict.tagTypeOf("1")).toBeUndefined();
  });
});
