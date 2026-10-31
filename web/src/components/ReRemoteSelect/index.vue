<script setup lang="ts">
import { computed, ref } from "vue";
import type { LookupOption } from "@/api/types";

defineOptions({ name: "ReRemoteSelect" });

/**
 * 关联字段下拉：输入关键字远程搜索，v-model 为所选记录的 ID。
 * `<ReRemoteSelect v-model="form.customerId" :fetch="listCustomerOptions" :label="row?.customerName" />`
 */
const id = defineModel<string | null>({ default: null });
const props = withDefaults(
  defineProps<{
    fetch: (keyword: string) => Promise<LookupOption[]>;
    /** 当前值的显示文字（编辑时由列表行带入），避免先显示 ID */
    label?: string;
    placeholder?: string;
  }>(),
  { label: "", placeholder: "输入关键字搜索" }
);

const loading = ref(false);
const found = ref<LookupOption[]>([]);
let seq = 0;

/** 搜索结果加上当前值，保证已选项能显示文字 */
const options = computed<LookupOption[]>(() => {
  const list = found.value;
  if (!id.value || !props.label || list.some(o => o.id === id.value)) {
    return list;
  }
  return [{ id: id.value, label: props.label }, ...list];
});

async function search(keyword: string): Promise<void> {
  const current = ++seq;
  loading.value = true;
  try {
    const list = (await props.fetch(keyword)) ?? [];
    if (current === seq) found.value = list;
  } catch {
    if (current === seq) found.value = [];
  } finally {
    if (current === seq) loading.value = false;
  }
}

function onVisible(open: boolean): void {
  if (open && found.value.length === 0) search("");
}
</script>

<template>
  <el-select
    v-model="id"
    filterable
    remote
    clearable
    :remote-method="search"
    :loading="loading"
    :placeholder="props.placeholder"
    @visible-change="onVisible"
    @clear="id = null"
  >
    <el-option
      v-for="o in options"
      :key="o.id"
      :label="o.label"
      :value="o.id"
    />
  </el-select>
</template>
