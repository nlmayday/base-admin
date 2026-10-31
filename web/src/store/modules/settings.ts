import { defineStore } from "pinia";
import { type setType, store, getConfig } from "../utils";
import { setConfig } from "@/config";

export const useSettingStore = defineStore("pure-setting", {
  state: (): setType => ({
    title: getConfig().Title,
    logo: "",
    fixedHeader: getConfig().FixedHeader,
    hiddenSideBar: getConfig().HiddenSideBar
  }),
  getters: {
    getTitle(state) {
      return state.title;
    },
    getFixedHeader(state) {
      return state.fixedHeader;
    },
    getHiddenSideBar(state) {
      return state.hiddenSideBar;
    }
  },
  actions: {
    CHANGE_SETTING({ key, value }) {
      if (Reflect.has(this, key)) {
        this[key] = value;
      }
    },
    changeSetting(data) {
      this.CHANGE_SETTING(data);
    },
    /** 应用「基础设置」里的站点名称与 Logo；空值保留默认 */
    applyBranding({
      siteName,
      logoUrl
    }: {
      siteName?: string;
      logoUrl?: string;
    }) {
      const name = siteName?.trim();
      if (name && name !== this.title) {
        const previous = this.title;
        this.title = name;
        setConfig({ Title: name });
        // 标题格式为「页面 | 站点名」，立即替换站点名部分
        if (previous && document.title.endsWith(`| ${previous}`)) {
          document.title = `${document.title.slice(0, -previous.length)}${name}`;
        }
      }
      this.logo = logoUrl?.trim() ?? "";
    }
  }
});

export function useSettingStoreHook() {
  return useSettingStore(store);
}
