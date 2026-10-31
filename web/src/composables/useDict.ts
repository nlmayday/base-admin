import { computed, ref, shallowRef, watch, type Ref } from "vue";
import {
  getDictOptions,
  type DictOption,
  type DictTagType
} from "@/api/system/dict";

/** 进程内缓存的有效期：字典很少变化，但管理员修改后刷新页面即可看到 */
const CACHE_TTL_MS = 5 * 60 * 1000;

type CacheEntry = { items: DictOption[]; expires: number };

const cache = new Map<string, CacheEntry>();
const inflight = new Map<string, Promise<DictOption[]>>();

async function loadDict(code: string): Promise<DictOption[]> {
  const hit = cache.get(code);
  if (hit && hit.expires > Date.now()) return hit.items;
  const pending = inflight.get(code);
  if (pending) return pending;
  const request = getDictOptions(code)
    .then(items => {
      const list = items ?? [];
      cache.set(code, { items: list, expires: Date.now() + CACHE_TTL_MS });
      return list;
    })
    .finally(() => inflight.delete(code));
  inflight.set(code, request);
  return request;
}

/** 清除字典缓存（不传 code 时清除全部），字典管理页保存后调用 */
export function invalidateDict(code?: string): void {
  if (code === undefined) cache.clear();
  else cache.delete(code);
}

export type UseDict = {
  items: Ref<DictOption[]>;
  loading: Ref<boolean>;
  /** 按值取标签；未知值原样返回 */
  labelOf: (value: string | number | null | undefined) => string;
  /** 按值取 el-tag 的 type；未设置时返回 undefined（默认样式） */
  tagTypeOf: (
    value: string | number | null | undefined
  ) => Exclude<DictTagType, ""> | undefined;
  reload: () => Promise<void>;
};

/**
 * 读取数据字典的启用项，用于下拉框和标签：
 * `const { items, labelOf, tagTypeOf } = useDict("order_status")`
 */
export function useDict(code: string | Ref<string>): UseDict {
  const codeRef = computed(() =>
    typeof code === "string" ? code : code.value
  );
  const items = shallowRef<DictOption[]>([]);
  const loading = ref(false);
  const byValue = computed(
    () => new Map(items.value.map(item => [item.value, item] as const))
  );

  async function load(force = false): Promise<void> {
    const current = codeRef.value;
    if (!current) {
      items.value = [];
      return;
    }
    if (force) invalidateDict(current);
    loading.value = true;
    try {
      const list = await loadDict(current);
      if (current === codeRef.value) items.value = list;
    } catch {
      // 错误提示已由 http 拦截器统一处理
      if (current === codeRef.value) items.value = [];
    } finally {
      loading.value = false;
    }
  }

  watch(codeRef, () => load(), { immediate: true });

  function labelOf(value: string | number | null | undefined): string {
    if (value === null || value === undefined || value === "") return "-";
    const key = String(value);
    return byValue.value.get(key)?.label ?? key;
  }

  function tagTypeOf(
    value: string | number | null | undefined
  ): Exclude<DictTagType, ""> | undefined {
    if (value === null || value === undefined) return undefined;
    return byValue.value.get(String(value))?.tagType || undefined;
  }

  return { items, loading, labelOf, tagTypeOf, reload: () => load(true) };
}

/**
 * 固定选项（如代码生成器从字段注释「类型：1=通知,2=公告」解析出的选项），
 * 返回与 useDict 相同的结构，页面写法一致
 */
export function staticDict(
  options: ReadonlyArray<{ value: string; label: string }>
): UseDict {
  const items = shallowRef<DictOption[]>(
    options.map(o => ({ ...o, tagType: "" }))
  );
  const byValue = new Map(options.map(o => [o.value, o.label] as const));
  return {
    items,
    loading: ref(false),
    labelOf: value =>
      value === null || value === undefined || value === ""
        ? "-"
        : (byValue.get(String(value)) ?? String(value)),
    tagTypeOf: () => undefined,
    reload: async () => undefined
  };
}
