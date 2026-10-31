<script setup lang="ts">
import { computed } from "vue";
import type { RouteDoc } from "@/api/devtools/apidoc";
import { ACCESS_META } from "../utils";

defineOptions({ name: "DevtoolsAccessTags" });

const props = defineProps<{ route: RouteDoc }>();

const meta = computed(() =>
  props.route.access === "permission" ? null : ACCESS_META[props.route.access]
);
</script>

<template>
  <div class="flex flex-wrap items-center gap-1">
    <el-tooltip v-if="meta" :content="meta.tip" placement="top">
      <el-tag :type="meta.tag">{{ meta.label }}</el-tag>
    </el-tooltip>
    <template v-else>
      <span
        v-if="route.permissions.length > 1"
        class="text-xs text-[var(--el-text-color-secondary)]"
      >
        任一权限：
      </span>
      <el-tooltip
        v-for="p in route.permissions"
        :key="p.code"
        :content="p.code"
        placement="top"
      >
        <el-tag type="success">{{ p.name || p.code }}</el-tag>
      </el-tooltip>
    </template>
    <el-tooltip
      v-if="route.csrf"
      content="POST/PUT/PATCH/DELETE 需在 X-XSRF-TOKEN 请求头中携带 XSRF-TOKEN Cookie 的值"
      placement="top"
    >
      <el-tag type="info" effect="plain">需 XSRF</el-tag>
    </el-tooltip>
    <el-tooltip
      v-if="route.sensitive"
      content="敏感读取，会记录到操作日志"
      placement="top"
    >
      <el-tag type="warning" effect="plain">敏感操作</el-tag>
    </el-tooltip>
  </div>
</template>
