import { getPublicSettings } from "@/api/system/settings";
import { useSettingStoreHook } from "@/store/modules/settings";

/**
 * 读取「基础设置」中的站点名称与 Logo 并应用到全局。
 * 失败时保留 platform-config.json 中的默认值，不影响登录。
 */
export async function loadBranding(): Promise<void> {
  try {
    useSettingStoreHook().applyBranding(await getPublicSettings());
  } catch {
    // 公共设置不可用时沿用默认品牌
  }
}
