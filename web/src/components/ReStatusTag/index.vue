<script setup lang="ts">
import type { Status } from "@/api/types";

defineOptions({ name: "ReStatusTag" });

defineProps<{ status: Status }>();
</script>

<template>
  <el-tag :type="status === 1 ? 'success' : 'info'" disable-transitions>
    {{ status === 1 ? "启用" : "停用" }}
  </el-tag>
</template>
