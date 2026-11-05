import { describe, expect, it, vi } from "vitest";
import { DEFAULT_PAGE_SIZE, usePagedTable } from "./usePagedTable";

const page = (n: number) => ({
  list: Array.from({ length: n }, (_, i) => ({ id: String(i) })),
  total: 42,
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE
});

describe("usePagedTable", () => {
  it("loads, pages and resets to the first page on search", async () => {
    const fetcher = vi.fn().mockResolvedValue(page(3));
    const t = usePagedTable(fetcher);
    await t.reload();
    expect(fetcher).toHaveBeenLastCalledWith({
      page: 1,
      pageSize: DEFAULT_PAGE_SIZE
    });
    expect(t.rows.value).toHaveLength(3);
    expect(t.pagination.total).toBe(42);
    expect(t.loading.value).toBe(false);

    t.onCurrentChange(3);
    expect(fetcher).toHaveBeenLastCalledWith({ page: 3, pageSize: 20 });
    t.onSizeChange(50);
    expect(fetcher).toHaveBeenLastCalledWith({ page: 1, pageSize: 50 });
    t.pagination.currentPage = 2;
    await t.search();
    expect(fetcher).toHaveBeenLastCalledWith({ page: 1, pageSize: 50 });
  });

  it("keeps whether the total was capped", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce({ ...page(2), total: 10000, totalCapped: true })
      .mockResolvedValueOnce(page(2));
    const t = usePagedTable(fetcher);
    expect(t.pagination.totalCapped).toBe(false);
    await t.reload();
    expect(t.pagination.total).toBe(10000);
    expect(t.pagination.totalCapped).toBe(true);
    await t.reload();
    expect(t.pagination.totalCapped).toBe(false);
  });

  it("clears the rows when loading fails", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(page(2))
      .mockRejectedValueOnce(new Error("x"));
    const t = usePagedTable(fetcher);
    await t.reload();
    await t.reload();
    expect(t.rows.value).toEqual([]);
    expect(t.loading.value).toBe(false);
  });
});
