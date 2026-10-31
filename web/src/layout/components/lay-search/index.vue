<script setup lang="ts">
import { defineAsyncComponent, ref } from "vue";
import { useBoolean } from "../../hooks/useBoolean";

// 搜索弹窗带拼音匹配库（pinyin-pro，约 300 KB），首次打开时才加载
const SearchModal = defineAsyncComponent(
  () => import("./components/SearchModal.vue")
);

const { bool: show, toggle } = useBoolean();
const opened = ref(false);
function handleSearch() {
  opened.value = true;
  toggle();
}
</script>

<template>
  <div>
    <div
      class="search-container w-[40px] h-[48px] flex-c cursor-pointer navbar-bg-hover"
      @click="handleSearch"
    >
      <IconifyIconOffline icon="ri/search-line" />
    </div>
    <SearchModal v-if="opened" v-model:value="show" />
  </div>
</template>
