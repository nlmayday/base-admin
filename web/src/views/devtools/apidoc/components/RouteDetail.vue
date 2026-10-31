<script setup lang="ts">
import { computed } from "vue";
import type { RouteDoc } from "@/api/devtools/apidoc";
import CodeBlock from "../../shared/CodeBlock.vue";
import AccessTags from "./AccessTags.vue";
import SchemaTree from "./SchemaTree.vue";
import { exampleURL, methodTag, toJSON } from "../utils";

defineOptions({ name: "DevtoolsRouteDetail" });

const props = defineProps<{ route: RouteDoc; moduleTitle: string }>();

const requestJSON = computed(() => toJSON(props.route.requestExample));
const responseJSON = computed(() => toJSON(props.route.responseExample));
const url = computed(() => `${props.route.method} ${exampleURL(props.route)}`);
const isJSON = computed(() => props.route.responseType === "application/json");
</script>

<template>
  <div class="route-detail">
    <div class="flex flex-wrap items-center gap-2">
      <el-tag :type="methodTag(route.method)" size="large" effect="dark">
        {{ route.method }}
      </el-tag>
      <span class="route-detail__path">{{ route.path }}</span>
    </div>
    <h3 class="mt-3 mb-1 text-lg font-medium">
      {{ route.summary || "未命名接口" }}
    </h3>
    <p
      v-if="route.description"
      class="mb-2 text-sm text-[var(--el-text-color-regular)]"
    >
      {{ route.description }}
    </p>
    <el-descriptions :column="2" border size="small" class="mt-3">
      <el-descriptions-item label="访问控制" :span="2">
        <AccessTags :route="route" />
      </el-descriptions-item>
      <el-descriptions-item label="所属模块">
        {{ moduleTitle }}（{{ route.module }}）
      </el-descriptions-item>
      <el-descriptions-item label="operationId">
        <span class="font-mono">{{ route.operationId }}</span>
      </el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">请求参数</el-divider>
    <el-table
      v-if="route.parameters.length > 0"
      :data="route.parameters"
      border
      size="small"
    >
      <el-table-column label="名称" min-width="140">
        <template #default="{ row }">
          <span class="font-mono">{{ row.name }}</span>
        </template>
      </el-table-column>
      <el-table-column label="位置" width="80" align="center">
        <template #default="{ row }">
          <el-tag
            size="small"
            :type="row.in === 'path' ? 'warning' : 'info'"
            effect="plain"
          >
            {{ row.in }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="类型" min-width="140">
        <template #default="{ row }">
          <span class="font-mono text-[var(--el-color-primary)]">
            {{ row.type }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="必填" width="70" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.required" size="small" type="danger" effect="plain">
            是
          </el-tag>
          <span v-else class="text-[var(--el-text-color-placeholder)]">-</span>
        </template>
      </el-table-column>
      <el-table-column label="约束" min-width="140">
        <template #default="{ row }">
          {{ row.constraints.join("，") || "-" }}
        </template>
      </el-table-column>
      <el-table-column label="说明" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.description || "-" }}</template>
      </el-table-column>
    </el-table>
    <div v-else class="route-detail__none">无路径或查询参数</div>

    <el-divider content-position="left">请求体</el-divider>
    <SchemaTree v-if="route.requestBody" :fields="route.requestBody" />
    <div v-else class="route-detail__none">无请求体</div>

    <el-divider content-position="left">响应</el-divider>
    <template v-if="isJSON && route.responseExample">
      <div class="mb-2 text-xs text-[var(--el-text-color-secondary)]">
        响应统一包裹在
        <span class="font-mono">{code, message, data, requestId}</span>
        信封中，下表描述 <span class="font-mono">data</span>。
      </div>
      <SchemaTree v-if="route.response" :fields="route.response" />
      <div v-else class="route-detail__none">data 固定为 null</div>
    </template>
    <div v-else class="route-detail__none">
      直接返回 <span class="font-mono">{{ route.responseType || "-" }}</span>
      内容（不带 JSON 信封）
    </div>

    <el-divider content-position="left">示例</el-divider>
    <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
      <div class="min-w-0">
        <CodeBlock
          :code="url"
          language="typescript"
          title="请求地址"
          :line-numbers="false"
        />
        <CodeBlock
          v-if="requestJSON"
          :code="requestJSON"
          language="json"
          title="请求体示例"
          max-height="360px"
          class="mt-3"
        />
      </div>
      <div class="min-w-0">
        <CodeBlock
          v-if="responseJSON"
          :code="responseJSON"
          language="json"
          title="响应示例"
          max-height="420px"
        />
        <el-empty v-else description="无 JSON 响应示例" :image-size="60" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.route-detail__path {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 15px;
  word-break: break-all;
}

.route-detail__none {
  padding: 8px 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
