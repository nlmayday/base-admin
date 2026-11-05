<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { useNav } from "@/layout/hooks/useNav";

const props = defineProps({
  collapse: Boolean
});

const { title, getLogo } = useNav();

/** 站点名放不下时依次缩小字号，到最小仍放不下才显示省略号（悬停看全名） */
const TITLE_SIZES = [18, 16, 14];
const titleEl = ref<HTMLElement | null>(null);
const titleSize = ref(TITLE_SIZES[0]);

async function fitTitle() {
  for (const size of TITLE_SIZES) {
    titleSize.value = size;
    await nextTick();
    const el = titleEl.value;
    if (!el || el.scrollWidth <= el.clientWidth) return;
  }
}

watch([title, () => props.collapse, titleEl], fitTitle, { immediate: true });
</script>

<template>
  <div class="sidebar-logo-container" :class="{ collapses: collapse }">
    <transition name="sidebarLogoFade">
      <router-link
        v-if="collapse"
        key="collapse"
        :title="title"
        class="sidebar-logo-link"
        to="/"
      >
        <img :src="getLogo()" alt="logo" />
        <span class="sidebar-title">{{ title }}</span>
      </router-link>
      <router-link
        v-else
        key="expand"
        :title="title"
        class="sidebar-logo-link"
        to="/"
      >
        <img :src="getLogo()" alt="logo" />
        <span
          ref="titleEl"
          class="sidebar-title"
          :style="{ fontSize: `${titleSize}px` }"
          >{{ title }}</span
        >
      </router-link>
    </transition>
  </div>
</template>

<style lang="scss" scoped>
.sidebar-logo-container {
  position: relative;
  width: 100%;
  height: 48px;
  overflow: hidden;

  .sidebar-logo-link {
    display: flex;
    flex-wrap: nowrap;
    align-items: center;
    height: 100%;
    padding-left: 10px;

    img {
      display: inline-block;
      height: 32px;
    }

    .sidebar-title {
      display: inline-block;
      height: 32px;
      margin: 2px 0 0 12px;
      overflow: hidden;
      text-overflow: ellipsis;
      font-size: 18px;
      font-weight: 600;
      line-height: 32px;
      color: var(--pure-theme-sub-menu-active-text);
      white-space: nowrap;
    }
  }
}
</style>
