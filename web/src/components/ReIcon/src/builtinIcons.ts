// 菜单图标（后端以 "ri:xxx" 返回）预先注册到在线图标组件的本地缓存，内网部署时
// 无需访问 api.iconify.design；数据由 scripts/gen-menu-icons.mjs 生成，菜单
// 图标选择器也只提供这些图标。管理员手填的其他图标仍按需在线加载。
import { addCollection } from "@iconify/vue";
import { MENU_ICON_DATA } from "./menuIcons";

addCollection(MENU_ICON_DATA);
