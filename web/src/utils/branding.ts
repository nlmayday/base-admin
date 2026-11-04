import { getPublicSettings } from "@/api/system/settings";
import { useSettingStoreHook } from "@/store/modules/settings";
import { setDefaultPageSize } from "@/utils/pageSize";

/**
 * 读取「基础设置」中的站点名称、Logo 与列表默认每页条数并应用到全局。
 * 失败时保留 platform-config.json 中的默认值，不影响登录。
 */
export async function loadBranding(): Promise<void> {
  try {
    const settings = await getPublicSettings();
    useSettingStoreHook().applyBranding(settings);
    setDefaultPageSize(settings?.pageSize);
  } catch {
    // 公共设置不可用时沿用默认品牌
  }
}
