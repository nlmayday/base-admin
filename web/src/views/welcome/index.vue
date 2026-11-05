<script setup lang="ts">
import { computed } from "vue";
import { hasPerm } from "@/utils/permission";
import GreetingCard from "./components/GreetingCard.vue";
import StatTiles from "./components/StatTiles.vue";
import QuickLinks from "./components/QuickLinks.vue";
import SystemCard from "./components/SystemCard.vue";
import AuditStatsCard from "@/views/audit/components/AuditStatsCard.vue";

defineOptions({ name: "Welcome" });

const canLoginLog = computed(() => hasPerm("audit:login-log:view"));
const canOperationLog = computed(() => hasPerm("audit:operation-log:view"));
const canMonitor = computed(() => hasPerm("ops:monitor:view"));
</script>

<template>
  <div class="main dashboard">
    <GreetingCard />
    <StatTiles />

    <el-row :gutter="12">
      <el-col :xs="24" :lg="canMonitor ? 15 : 24" class="col">
        <QuickLinks />
      </el-col>
      <el-col v-if="canMonitor" :xs="24" :lg="9" class="col">
        <SystemCard />
      </el-col>
    </el-row>

    <template v-if="canOperationLog || canLoginLog">
      <div class="section-title card-title">近 7 天动态</div>
      <AuditStatsCard v-if="canOperationLog" kind="operation" />
      <AuditStatsCard v-if="canLoginLog" kind="login" />
    </template>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dashboard :deep(.el-card.mb-3) {
  margin-bottom: 0;
}

.col {
  margin-bottom: 0;
}

@media (width <= 1200px) {
  .col + .col {
    margin-top: 12px;
  }
}

.section-title {
  margin-top: 4px;
}
</style>
