/** 通用分页参数 */
export type PageQuery = {
  page: number;
  pageSize: number;
};

/** 通用分页结果 */
export type Page<T> = {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
  /** 大表只数到上限：total 即上限，后面还有更多行，只能翻到第 total 条 */
  totalCapped?: boolean;
};

/** 1 = 启用，2 = 停用 */
export type Status = 1 | 2;

export const STATUS_ENABLED: Status = 1;
export const STATUS_DISABLED: Status = 2;

export const STATUS_OPTIONS: ReadonlyArray<{ label: string; value: Status }> = [
  { label: "启用", value: STATUS_ENABLED },
  { label: "停用", value: STATUS_DISABLED }
];

/** 下拉选项（角色、岗位） */
export type CodeOption = { id: string; code: string; name: string };

/** 去掉空字符串 / null / undefined 的查询参数，避免把空筛选条件发给后端 */
export function compactParams<T extends object>(params: T): Partial<T> {
  return Object.fromEntries(
    Object.entries(params).filter(
      ([, v]) => v !== "" && v !== null && v !== undefined
    )
  ) as Partial<T>;
}

/** 关联字段的下拉选项（生成模块的 /options/<字段> 接口） */
export type LookupOption = { id: string; label: string };
