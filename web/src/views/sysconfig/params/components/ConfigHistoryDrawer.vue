<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { formatDateTime } from "@/utils/format";
import {
  listConfigChanges,
  type Config,
  type ConfigChange
} from "@/api/system/config";

defineOptions({ name: "ConfigHistoryDrawer" });

const props = defineProps<{ config: Config | null }>();
const visible = defineModel<boolean>({ required: true });

const PAGE_SIZE = 20;

const loading = ref(false);
const changes = ref<ConfigChange[]>([]);
const total = ref(0);
const page = ref(1);
const hasMore = computed(() => changes.value.length < total.value);

async function load(reset: boolean): Promise<void> {
  const config = props.config;
  if (!config) return;
  if (reset) {
    page.value = 1;
    changes.value = [];
    total.value = 0;
  }
  loading.value = true;
  try {
    const data = await listConfigChanges(config.id, {
      page: page.value,
      pageSize: PAGE_SIZE
    });
    changes.value = [...changes.value, ...(data?.list ?? [])];
    total.value = data?.total ?? 0;
  } catch {
    // 错误提示已由 http 拦截器统一处理
  } finally {
    loading.value = false;
  }
}

function loadMore(): void {
  page.value += 1;
  load(false);
}

watch(visible, open => {
  if (open) load(true);
});
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="config ? `变更记录 · ${config.name}` : '变更记录'"
    size="520px"
    destroy-on-close
  >
    <div
      v-if="config"
      class="mb-4 text-sm text-[var(--el-text-color-secondary)]"
    >
      <span class="font-mono">{{ config.key }}</span>
      <span class="ml-2">共 {{ total }} 条记录</span>
    </div>
    <div v-loading="loading && changes.length === 0" class="min-h-[120px]">
      <el-empty
        v-if="!loading && changes.length === 0"
        description="暂无变更记录"
        :image-size="80"
      />
      <el-timeline v-else>
        <el-timeline-item
          v-for="c in changes"
          :key="c.id"
          :timestamp="formatDateTime(c.changedAt)"
          :type="c.oldValue === null ? 'success' : 'primary'"
          placement="top"
        >
          <div class="mb-2 text-sm">
            <span class="font-medium">{{ c.changedByName || "系统" }}</span>
            <span class="ml-1 text-[var(--el-text-color-secondary)]">
              {{ c.oldValue === null ? "创建了参数" : "修改了参数值" }}
            </span>
          </div>
          <div
            v-if="c.oldValue !== null"
            class="change-value change-value--old"
          >
            {{ c.oldValue === "" ? "（空）" : c.oldValue }}
          </div>
          <div class="change-value change-value--new">
            {{ c.newValue === "" ? "（空）" : c.newValue }}
          </div>
        </el-timeline-item>
      </el-timeline>
      <div v-if="hasMore" class="text-center">
        <el-button :loading="loading" link type="primary" @click="loadMore">
          加载更多
        </el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped>
.change-value {
  max-height: 200px;
  padding: 6px 10px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
  white-space: pre-wrap;
  border-radius: 4px;
}

.change-value + .change-value {
  margin-top: 4px;
}

.change-value--old {
  color: var(--el-color-danger);
  text-decoration: line-through;
  background: var(--el-color-danger-light-9);
}

.change-value--new {
  color: var(--el-color-success-dark-2);
  background: var(--el-color-success-light-9);
}
</style>
