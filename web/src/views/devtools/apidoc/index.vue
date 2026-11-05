<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  downloadOpenAPI,
  getApiDoc,
  type ModuleSummary,
  type RouteDoc
} from "@/api/devtools/apidoc";
import RouteNav from "./components/RouteNav.vue";
import RouteDetail from "./components/RouteDetail.vue";
import { routeKey } from "./utils";

defineOptions({ name: "DevtoolsApidoc" });

const loading = ref(false);
const downloading = ref(false);
const modules = ref<ModuleSummary[]>([]);
const routes = ref<RouteDoc[]>([]);
const activeKey = ref("");

const active = computed(
  () => routes.value.find(r => routeKey(r) === activeKey.value) ?? null
);
const activeModuleTitle = computed(
  () =>
    modules.value.find(m => m.name === active.value?.module)?.title ??
    active.value?.module ??
    ""
);
const stats = computed(() => ({
  modules: modules.value.length,
  routes: routes.value.length,
  public: routes.value.filter(r => r.access === "public").length
}));

async function reload(): Promise<void> {
  loading.value = true;
  try {
    const doc = await getApiDoc();
    modules.value = doc?.modules ?? [];
    routes.value = doc?.routes ?? [];
    if (!active.value && routes.value.length > 0) {
      activeKey.value = routeKey(routes.value[0]);
    }
  } catch {
    modules.value = [];
    routes.value = [];
  } finally {
    loading.value = false;
  }
}

function select(route: RouteDoc): void {
  activeKey.value = routeKey(route);
}

async function onDownload(): Promise<void> {
  downloading.value = true;
  try {
    await downloadOpenAPI();
  } catch {
    // 错误提示已由 http 拦截器处理
  } finally {
    downloading.value = false;
  }
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="py-3!">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-4 text-sm">
          <span class="card-title">接口文档</span>
          <span class="text-[var(--el-text-color-secondary)]">
            {{ stats.modules }} 个模块 · {{ stats.routes }} 个接口 ·
            {{ stats.public }} 个公开接口
          </span>
          <span class="text-xs text-[var(--el-text-color-secondary)]">
            由后端模块路由注册表实时生成，前缀 /api/v1
          </span>
        </div>
        <div>
          <el-button :loading="loading" @click="reload">刷新</el-button>
          <el-button type="primary" :loading="downloading" @click="onDownload">
            下载 openapi.json
          </el-button>
        </div>
      </div>
    </el-card>
    <div class="apidoc flex gap-3">
      <el-card
        shadow="never"
        class="apidoc__nav w-[320px] shrink-0"
        body-class="h-full! p-3!"
      >
        <RouteNav
          :modules="modules"
          :routes="routes"
          :active-key="activeKey"
          :loading="loading"
          @select="select"
        />
      </el-card>
      <el-card shadow="never" class="min-w-0 flex-1">
        <RouteDetail
          v-if="active"
          :key="activeKey"
          :route="active"
          :module-title="activeModuleTitle"
        />
        <el-empty v-else description="从左侧选择一个接口查看详情" />
      </el-card>
    </div>
  </div>
</template>

<style scoped>
.apidoc {
  align-items: flex-start;
}

.apidoc__nav {
  position: sticky;
  top: 0;
  height: calc(100vh - 190px);
  min-height: 420px;
}
</style>
