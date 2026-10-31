<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { message } from "@/utils/message";
import { formatDateTime } from "@/utils/format";
import {
  getOperationLog,
  type OperationLogDetail
} from "@/api/audit/operationLog";
import {
  durationClass,
  methodTag,
  moduleLabel,
  prettyJson,
  statusTag
} from "../../utils";

defineOptions({ name: "OperationLogDrawer" });

const props = defineProps<{ logId: string | null }>();
const visible = defineModel<boolean>({ required: true });

const loading = ref(false);
const detail = ref<OperationLogDetail | null>(null);

const body = computed(() => prettyJson(detail.value?.requestBody ?? ""));
const operator = computed(() => {
  const d = detail.value;
  if (!d) return "-";
  if (!d.userId) return "匿名（未登录）";
  if (!d.username) return `用户 #${d.userId}（已删除）`;
  return d.nickname ? `${d.username}（${d.nickname}）` : d.username;
});

async function load(id: string): Promise<void> {
  loading.value = true;
  detail.value = null;
  try {
    detail.value = await getOperationLog(id);
  } catch {
    visible.value = false; // 错误提示已由 http 拦截器处理
  } finally {
    loading.value = false;
  }
}

watch(
  () => [visible.value, props.logId] as const,
  ([open, id]) => {
    if (open && id) load(id);
  },
  { immediate: true }
);

async function copyBody(): Promise<void> {
  try {
    await navigator.clipboard.writeText(body.value);
    message("已复制", { type: "success" });
  } catch {
    message("复制失败，请手动选择文本", { type: "warning" });
  }
}
</script>

<template>
  <el-drawer v-model="visible" title="操作详情" size="640px" destroy-on-close>
    <div v-loading="loading" class="min-h-[200px]">
      <template v-if="detail">
        <div class="mb-4 flex flex-wrap items-center gap-2">
          <el-tag :type="methodTag(detail.method)" disable-transitions>
            {{ detail.method }}
          </el-tag>
          <span class="text-base font-medium">
            {{ detail.summary || "未命名操作" }}
          </span>
          <el-tag
            :type="detail.success ? 'success' : 'danger'"
            effect="dark"
            size="small"
            disable-transitions
          >
            {{ detail.success ? "成功" : "失败" }}
          </el-tag>
        </div>

        <el-alert
          v-if="detail.errorMessage"
          type="error"
          :title="detail.errorMessage"
          :description="detail.code ? `业务码 ${detail.code}` : undefined"
          :closable="false"
          show-icon
          class="mb-4"
        />

        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="操作人" :span="2">
            {{ operator }}
          </el-descriptions-item>
          <el-descriptions-item label="模块">
            {{ moduleLabel(detail.module) }}
          </el-descriptions-item>
          <el-descriptions-item label="时间">
            {{ formatDateTime(detail.createdAt) }}
          </el-descriptions-item>
          <el-descriptions-item label="请求路径" :span="2">
            <span class="font-mono text-xs break-all">{{ detail.path }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="路由" :span="2">
            <span class="font-mono text-xs break-all">{{
              detail.route || "-"
            }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="HTTP 状态">
            <el-tag
              :type="statusTag(detail.status)"
              size="small"
              effect="plain"
              disable-transitions
            >
              {{ detail.status }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="业务码">
            {{ detail.code }}
          </el-descriptions-item>
          <el-descriptions-item label="耗时">
            <span :class="durationClass(detail.durationMs)" class="font-medium">
              {{ detail.durationMs }} ms
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="IP">
            {{ detail.ip || "-" }}
          </el-descriptions-item>
          <el-descriptions-item label="请求 ID" :span="2">
            <span class="font-mono text-xs">{{ detail.requestId || "-" }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="User-Agent" :span="2">
            <span class="text-xs break-all">{{ detail.userAgent || "-" }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <div class="mt-5 mb-2 flex items-center justify-between">
          <span class="font-medium">请求体</span>
          <el-button v-if="body" link type="primary" @click="copyBody">
            复制
          </el-button>
        </div>
        <pre v-if="body" class="request-body">{{ body }}</pre>
        <el-empty v-else description="无请求体" :image-size="60" />
        <p class="mt-2 text-xs text-[var(--el-text-color-secondary)]">
          仅记录 4 KB 以内的 JSON 请求体；password、token、secret、captcha
          等字段已脱敏为 ***。
        </p>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.request-body {
  max-height: 360px;
  padding: 12px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-primary);
  word-break: break-all;
  white-space: pre-wrap;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
}
</style>
