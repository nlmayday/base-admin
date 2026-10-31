// 生成菜单图标数据：从 @iconify/json 中挑出常用的 Remix 线性图标，写入
// src/components/ReIcon/src/menuIcons.ts。菜单图标选择器和侧边栏都用这份
// 离线数据，内网部署无需访问 api.iconify.design。
// 增删图标：修改下方 GROUPS 后运行 `node scripts/gen-menu-icons.mjs`，再用
// prettier 格式化生成的文件。
import { readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const set = JSON.parse(
  readFileSync(require.resolve("@iconify/json/json/ri.json"), "utf8")
);

const GROUPS = [
  [
    "系统",
    "home dashboard dashboard-3 settings-3 settings-4 equalizer list-settings tools apps apps-2 menu layout-grid function magic code-box terminal-box robot-2 bug database-2 server cloud computer hard-drive-2 git-branch route plug global"
  ],
  [
    "用户与权限",
    "user-3 user team group admin user-settings contacts account-circle shield-keyhole shield-user shield-check lock-password key-2 login-box organization-chart briefcase"
  ],
  [
    "内容与文件",
    "file-list-3 file-text file folder-3 folder-image image book-2 book-open article newspaper clipboard draft archive attachment upload-cloud-2 download-cloud-2"
  ],
  [
    "消息与日志",
    "notification-3 message-3 mail chat-3 megaphone history timer time calendar calendar-event alarm file-search search"
  ],
  [
    "业务",
    "shopping-cart shopping-bag store-2 money-cny-circle wallet-3 bank-card coupon gift truck map-pin building customer-service-2 service ticket bar-chart-box line-chart pie-chart stack price-tag-3 task todo checkbox-circle flag star heart links questionnaire survey inbox box-3"
  ]
];

const icons = {};
const groups = GROUPS.map(([label, names]) => {
  const list = [];
  for (const base of names.split(" ")) {
    const name = base === "organization-chart" ? base : `${base}-line`;
    const icon = set.icons[name];
    if (!icon) {
      console.warn(`跳过不存在的图标 ri:${name}`);
      continue;
    }
    icons[name] = icon;
    list.push(`ri:${name}`);
  }
  return { label, icons: list };
});

const out = `// 由 scripts/gen-menu-icons.mjs 生成，请勿手改。
import type { IconifyJSON } from "@iconify/vue";

export type MenuIconGroup = { label: string; icons: readonly string[] };

/** 图标选择器的分组，值为 "ri:xxx-line" */
export const MENU_ICON_GROUPS: readonly MenuIconGroup[] = ${JSON.stringify(groups)};

/** Remix 图标数据（${Object.keys(icons).length} 个） */
export const MENU_ICON_DATA: IconifyJSON = ${JSON.stringify({ prefix: "ri", width: set.width ?? 24, height: set.height ?? 24, icons })};
`;
writeFileSync(
  new URL("../src/components/ReIcon/src/menuIcons.ts", import.meta.url),
  out
);
console.log(`已生成 ${Object.keys(icons).length} 个菜单图标`);
