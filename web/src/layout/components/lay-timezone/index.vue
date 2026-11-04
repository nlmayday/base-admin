<script setup lang="ts">
import { computed } from "vue";
import {
  displayZone,
  localZoneLabel,
  setDisplayZone,
  type DisplayZone
} from "@/utils/timezone";

/** 顶栏的时间显示时区切换：浏览器时区 / UTC，选择记在浏览器里 */
const local = localZoneLabel();
const current = computed(() => (displayZone.value === "utc" ? "UTC" : local));

function onCommand(zone: DisplayZone): void {
  setDisplayZone(zone);
}
</script>

<template>
  <el-dropdown trigger="click" @command="onCommand">
    <span
      class="timezone-switch navbar-bg-hover select-none"
      data-testid="nav-timezone"
      title="时间显示时区"
    >
      {{ current }}
    </span>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          command="local"
          :class="{ 'is-active': displayZone === 'local' }"
        >
          本地时区（{{ local }}）
        </el-dropdown-item>
        <el-dropdown-item
          command="utc"
          :class="{ 'is-active': displayZone === 'utc' }"
        >
          UTC
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<style lang="scss" scoped>
.timezone-switch {
  display: flex;
  align-items: center;
  height: 48px;
  padding: 0 10px;
  font-size: 12px;
  color: var(--el-text-color-regular);
  cursor: pointer;
}

.is-active {
  color: var(--el-color-primary);
}
</style>
