/** 列表可选的每页条数；「基础设置」中的默认值必须是其中之一 */
export const PAGE_SIZES = [10, 20, 50, 100] as const;

/** 设置加载前（或加载失败时）的默认每页条数 */
export const FALLBACK_PAGE_SIZE = 20;

let current: number = FALLBACK_PAGE_SIZE;

/** 列表的默认每页条数（来自「基础设置 → 列表默认每页条数」） */
export function defaultPageSize(): number {
  return current;
}

/** 应用「基础设置」中的默认每页条数；不在可选范围内的值被忽略 */
export function setDefaultPageSize(size: number | undefined): void {
  if (size !== undefined && (PAGE_SIZES as readonly number[]).includes(size)) {
    current = size;
  }
}
