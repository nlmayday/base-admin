<script setup lang="ts">
import { computed, ref } from "vue";
import { useElementSize } from "@vueuse/core";
import { useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { usePermissionStoreHook } from "@/store/modules/permission";
import { useRenderIcon } from "@/components/ReIcon/src/hooks";
import { EXTERNAL_PATH_PREFIX } from "@/router/utils";

defineOptions({ name: "QuickLinks" });

/** 最多展示的行数，以及单个入口的最小宽度 + 间距（与样式一致） */
const MAX_ROWS = 2;
const LINK_MIN_WIDTH = 110;
const LINK_GAP = 8;
const HOME_PATH = "/welcome";

type MenuNode = {
  path: string;
  meta?: { title?: string; icon?: string; showLink?: boolean };
  children?: MenuNode[];
};

type Link = { path: string; title: string; group: string; icon?: string };

const router = useRouter();
const container = ref<HTMLElement | null>(null);
const { width } = useElementSize(container);
const { wholeMenus } = storeToRefs(usePermissionStoreHook());

/** 当前用户可见菜单中的页面，按菜单顺序取前若干个 */
const links = computed<Link[]>(() => {
  const out: Link[] = [];
  const walk = (nodes: MenuNode[], group: string, parentIcon?: string) => {
    for (const n of nodes) {
      if (n.meta?.showLink === false) continue;
      const icon = n.meta?.icon || parentIcon;
      if (n.children?.length) {
        walk(n.children, n.meta?.title ?? "", icon);
      } else if (
        n.path !== HOME_PATH &&
        !n.path.startsWith(EXTERNAL_PATH_PREFIX)
      ) {
        out.push({ path: n.path, title: n.meta?.title ?? n.path, group, icon });
      }
    }
  };
  walk(wholeMenus.value as MenuNode[], "");
  return out;
});

/** 只展示完整的行，避免最后一行只剩一两个入口 */
const visibleLinks = computed<Link[]>(() => {
  const columns = Math.max(
    1,
    Math.floor((width.value + LINK_GAP) / (LINK_MIN_WIDTH + LINK_GAP))
  );
  if (links.value.length <= columns) return links.value;
  const rows = Math.min(MAX_ROWS, Math.floor(links.value.length / columns));
  return links.value.slice(0, rows * columns);
});
</script>

<template>
  <el-card shadow="never" class="h-full">
    <template #header>
      <span class="font-medium">快捷入口</span>
    </template>
    <div v-if="links.length" ref="container" class="links">
      <button
        v-for="l in visibleLinks"
        :key="l.path"
        type="button"
        class="link"
        @click="router.push(l.path)"
      >
        <span class="link-icon">
          <component :is="useRenderIcon(l.icon || 'ri:apps-line')" />
        </span>
        <span class="truncate text-sm">{{ l.title }}</span>
        <span
          v-if="l.group"
          class="truncate text-xs text-[var(--el-text-color-secondary)]"
        >
          {{ l.group }}
        </span>
      </button>
    </div>
    <el-empty v-else description="暂无可用功能" :image-size="60" />
  </el-card>
</template>

<style scoped>
.links {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: 8px;
}

.link {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
  min-width: 0;
  padding: 12px 6px;
  color: var(--el-text-color-primary);
  cursor: pointer;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 8px;
  transition:
    background 0.2s,
    border-color 0.2s;
}

.link:hover,
.link:focus-visible {
  outline: none;
  background: var(--el-fill-color-light);
  border-color: var(--el-border-color-lighter);
}

.link-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  font-size: 20px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  border-radius: 10px;
}
</style>
