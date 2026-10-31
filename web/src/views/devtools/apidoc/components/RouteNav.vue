<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { ModuleSummary, RouteDoc } from "@/api/devtools/apidoc";
import { matchesKeyword, methodTag, routeKey } from "../utils";

defineOptions({ name: "DevtoolsRouteNav" });

const props = defineProps<{
  modules: ModuleSummary[];
  routes: RouteDoc[];
  activeKey: string;
  loading: boolean;
}>();
const emit = defineEmits<{ (e: "select", route: RouteDoc): void }>();

const keyword = ref("");
const expanded = ref<string[]>([]);

type Group = ModuleSummary & { routes: RouteDoc[] };

const groups = computed<Group[]>(() =>
  props.modules
    .map(m => ({
      ...m,
      routes: props.routes.filter(
        r => r.module === m.name && matchesKeyword(r, keyword.value)
      )
    }))
    .filter(g => g.routes.length > 0)
);

const matched = computed(() =>
  groups.value.reduce((n, g) => n + g.routes.length, 0)
);

// 搜索时展开所有命中的分组；首次加载时全部展开
watch(
  [() => props.modules, keyword],
  () => {
    expanded.value = groups.value.map(g => g.name);
  },
  { immediate: true }
);
</script>

<template>
  <div class="flex h-full flex-col">
    <el-input
      v-model="keyword"
      placeholder="搜索路径、名称或权限"
      clearable
      class="mb-2"
    />
    <div class="mb-2 text-xs text-[var(--el-text-color-secondary)]">
      共 {{ routes.length }} 个接口{{ keyword ? `，匹配 ${matched} 个` : "" }}
    </div>
    <el-scrollbar v-loading="loading" class="min-h-0 flex-1">
      <el-empty
        v-if="!loading && groups.length === 0"
        :description="keyword ? '没有匹配的接口' : '暂无接口'"
        :image-size="80"
      />
      <el-collapse v-model="expanded" class="route-nav">
        <el-collapse-item v-for="g in groups" :key="g.name" :name="g.name">
          <template #title>
            <span class="font-medium">{{ g.title }}</span>
            <span class="ml-1 text-xs text-[var(--el-text-color-secondary)]">
              {{ g.name }} · {{ g.routes.length }}
            </span>
          </template>
          <button
            v-for="r in g.routes"
            :key="routeKey(r)"
            type="button"
            class="route-nav__item"
            :class="{ 'is-active': routeKey(r) === activeKey }"
            @click="emit('select', r)"
          >
            <el-tag
              :type="methodTag(r.method)"
              size="small"
              effect="plain"
              class="route-nav__method"
            >
              {{ r.method }}
            </el-tag>
            <span class="min-w-0 flex-1 text-left">
              <span class="route-nav__summary">{{ r.summary || "-" }}</span>
              <span class="route-nav__path">{{ r.path }}</span>
            </span>
          </button>
        </el-collapse-item>
      </el-collapse>
    </el-scrollbar>
  </div>
</template>

<style scoped>
.route-nav {
  border-top: none;
}

.route-nav :deep(.el-collapse-item__content) {
  padding-bottom: 4px;
}

.route-nav__item {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  width: 100%;
  padding: 6px 8px;
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: 4px;
}

.route-nav__item:hover {
  background: var(--el-fill-color-light);
}

.route-nav__item.is-active {
  background: var(--el-color-primary-light-9);
}

.route-nav__method {
  flex-shrink: 0;
  width: 58px;
  margin-top: 1px;
}

.route-nav__summary {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
  color: var(--el-text-color-primary);
  white-space: nowrap;
}

.route-nav__path {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
</style>
