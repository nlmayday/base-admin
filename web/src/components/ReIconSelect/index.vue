<script setup lang="ts">
import { computed, ref } from "vue";
import { MENU_ICON_GROUPS } from "@/components/ReIcon/src/menuIcons";

defineOptions({ name: "ReIconSelect" });

/** 图标名，如 "ri:home-line"；空字符串表示不设置 */
const icon = defineModel<string>({ default: "" });

const visible = ref(false);
const keyword = ref("");
const group = ref(MENU_ICON_GROUPS[0]?.label ?? "");

/** 有关键字时跨分组搜索，否则显示当前分组 */
const shown = computed<readonly string[]>(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (kw) {
    const all = MENU_ICON_GROUPS.flatMap(g => g.icons);
    return [...new Set(all)].filter(name => name.includes(kw));
  }
  return MENU_ICON_GROUPS.find(g => g.label === group.value)?.icons ?? [];
});

function pick(name: string): void {
  icon.value = name;
  visible.value = false;
}
</script>

<template>
  <el-popover
    v-model:visible="visible"
    placement="bottom-start"
    :width="420"
    trigger="click"
  >
    <template #reference>
      <el-input
        v-model="icon"
        clearable
        placeholder="点击选择图标，也可输入 iconify 名称"
        data-testid="icon-select"
      >
        <template #prefix>
          <IconifyIconOnline v-if="icon" :icon="icon" />
        </template>
      </el-input>
    </template>
    <el-input
      v-model="keyword"
      size="small"
      clearable
      placeholder="搜索英文名，如 user、file、chart"
      class="mb-2"
    />
    <el-radio-group
      v-if="!keyword.trim()"
      v-model="group"
      size="small"
      class="mb-2"
    >
      <el-radio-button
        v-for="g in MENU_ICON_GROUPS"
        :key="g.label"
        :value="g.label"
      >
        {{ g.label }}
      </el-radio-button>
    </el-radio-group>
    <div class="icon-grid">
      <span
        v-for="name in shown"
        :key="name"
        :class="['icon-cell', { active: name === icon }]"
        :title="name"
        @click="pick(name)"
      >
        <IconifyIconOnline :icon="name" />
      </span>
    </div>
    <el-empty
      v-if="shown.length === 0"
      description="没有匹配的图标"
      :image-size="48"
    />
  </el-popover>
</template>

<style scoped>
.icon-grid {
  display: grid;
  grid-template-columns: repeat(10, 1fr);
  gap: 4px;
  max-height: 240px;
  overflow-y: auto;
}

.icon-cell {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 34px;
  font-size: 18px;
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: 4px;
}

.icon-cell:hover {
  color: var(--el-color-primary);
  background: var(--el-fill-color-light);
}

.icon-cell.active {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}
</style>
