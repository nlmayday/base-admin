import { reactive, ref, type Ref } from "vue";
import type { Page, PageQuery } from "@/api/types";

export type Pagination = {
  currentPage: number;
  pageSize: number;
  total: number;
};

export type UsePagedTable<T> = {
  loading: Ref<boolean>;
  rows: Ref<T[]>;
  pagination: Pagination;
  /** 重新加载当前页 */
  reload: () => Promise<void>;
  /** 回到第一页并加载（搜索条件变化时使用） */
  search: () => Promise<void>;
  onSizeChange: (size: number) => void;
  onCurrentChange: (page: number) => void;
};

export const DEFAULT_PAGE_SIZE = 20;

/** 通用分页表格状态：fetcher 收到 page/pageSize，返回 Page<T> */
export function usePagedTable<T>(
  fetcher: (query: PageQuery) => Promise<Page<T>>
): UsePagedTable<T> {
  const loading = ref(false);
  const rows = ref<T[]>([]) as Ref<T[]>;
  const pagination = reactive<Pagination>({
    currentPage: 1,
    pageSize: DEFAULT_PAGE_SIZE,
    total: 0
  });

  async function reload(): Promise<void> {
    loading.value = true;
    try {
      const data = await fetcher({
        page: pagination.currentPage,
        pageSize: pagination.pageSize
      });
      rows.value = data?.list ?? [];
      pagination.total = data?.total ?? 0;
    } catch {
      // 错误提示已由 http 拦截器统一处理
      rows.value = [];
    } finally {
      loading.value = false;
    }
  }

  function search(): Promise<void> {
    pagination.currentPage = 1;
    return reload();
  }

  function onSizeChange(size: number): void {
    pagination.pageSize = size;
    pagination.currentPage = 1;
    reload();
  }

  function onCurrentChange(page: number): void {
    pagination.currentPage = page;
    reload();
  }

  return {
    loading,
    rows,
    pagination,
    reload,
    search,
    onSizeChange,
    onCurrentChange
  };
}
