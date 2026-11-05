import { onMounted, watch } from "vue";
import { useRoute, type LocationQuery } from "vue-router";

/** 地址栏参数怎样读成筛选值 */
export type FilterKind = "string" | "number" | "boolean";

export type RouteFilterOptions = {
  /** 筛选表单的值（会被地址栏中的同名参数覆盖） */
  filter: Record<string, unknown>;
  /** 每个筛选参数的类型 */
  kinds: Record<string, FilterKind>;
  /** 日期范围：x 对应地址栏的 xStart、xEnd */
  ranges?: Record<string, string[] | null>;
  /** 恢复筛选默认值（地址栏带了参数时先清掉其他条件） */
  reset: () => void;
  /** 查询（回到第一页） */
  search: () => void;
};

function first(v: LocationQuery[string]): string | undefined {
  const s = Array.isArray(v) ? v[0] : v;
  return typeof s === "string" ? s : undefined;
}

/**
 * 地址栏参数 → 筛选值；返回是否带了任何筛选参数。
 * 数字参数不是数字时忽略，布尔参数 true / 1 为真。
 */
export function applyRouteQuery(
  query: LocationQuery,
  opts: Pick<RouteFilterOptions, "filter" | "kinds" | "ranges">
): boolean {
  let hit = false;
  for (const [key, kind] of Object.entries(opts.kinds)) {
    const v = first(query[key]);
    if (v === undefined) continue;
    if (kind === "number") {
      const n = Number(v);
      if (v.trim() === "" || !Number.isFinite(n)) continue;
      opts.filter[key] = n;
    } else if (kind === "boolean") {
      opts.filter[key] = v === "true" || v === "1";
    } else {
      opts.filter[key] = v;
    }
    hit = true;
  }
  for (const key of Object.keys(opts.ranges ?? {})) {
    const start = first(query[`${key}Start`]);
    const end = first(query[`${key}End`]);
    if (!start && !end) continue;
    opts.ranges![key] = [start ?? end!, end ?? start!];
    hit = true;
  }
  return hit;
}

/**
 * 列表页读地址栏参数：与筛选字段同名的参数（如 ?userId=1001）填入筛选
 * 并查询，参数变化时（从别的页面跳过来）重新查询；没有参数时正常加载。
 */
export function useRouteFilter(opts: RouteFilterOptions): void {
  const route = useRoute();
  const path = route.path;

  function load(query: LocationQuery): void {
    opts.reset();
    applyRouteQuery(query, opts);
    opts.search();
  }

  // 回到本页（标签页切换）而参数没变时不重查，保留手动改过的筛选
  let last = JSON.stringify(route.query);
  onMounted(() => load(route.query));
  watch(
    () => route.query,
    q => {
      if (route.path !== path) return; // 离开本页
      const now = JSON.stringify(q);
      if (now === last) return;
      last = now;
      load(q);
    }
  );
}
