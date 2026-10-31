<script setup lang="ts">
import { computed } from "vue";
import { useUserStoreHook } from "@/store/modules/user";
import { useNav } from "@/layout/hooks/useNav";
import { greetingOf, todayLabel } from "../greeting";

defineOptions({ name: "GreetingCard" });

const userStore = useUserStoreHook();
const { userAvatar, title } = useNav();
const now = new Date();

const user = computed(() => userStore.me?.user);
const displayName = computed(
  () => user.value?.nickname || user.value?.username || ""
);
const roles = computed(() => userStore.me?.roles ?? []);
</script>

<template>
  <el-card shadow="never" class="greeting" body-class="greeting-body">
    <div class="flex items-center gap-4">
      <el-avatar :size="56" :src="userAvatar" class="shrink-0" />
      <div class="min-w-0">
        <h2 class="truncate text-lg font-semibold">
          {{ greetingOf(now.getHours()) }}，{{ displayName }}
        </h2>
        <div
          class="mt-1 flex flex-wrap items-center gap-2 text-sm text-[var(--el-text-color-secondary)]"
        >
          <span v-if="user?.deptName">{{ user.deptName }}</span>
          <el-tag
            v-for="r in roles"
            :key="r.id"
            size="small"
            effect="plain"
            round
          >
            {{ r.name }}
          </el-tag>
        </div>
      </div>
    </div>
    <div class="text-right">
      <div class="text-sm text-[var(--el-text-color-secondary)]">
        {{ todayLabel(now) }}
      </div>
      <div class="mt-1 text-base font-medium">欢迎使用 {{ title }}</div>
    </div>
  </el-card>
</template>

<style scoped>
.greeting {
  background: linear-gradient(
    120deg,
    var(--el-color-primary-light-9) 0%,
    var(--el-bg-color) 70%
  );
}

:deep(.greeting-body) {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
}
</style>
