import { getConfig } from "@/config";
import NProgress from "@/utils/progress";
import { buildHierarchyTree } from "@/utils/tree";
import remainingRouter from "./modules/remaining";
import { useUserStoreHook } from "@/store/modules/user";
import { useMultiTagsStoreHook } from "@/store/modules/multiTags";
import { usePermissionStoreHook } from "@/store/modules/permission";
import { isUrl, openLink, cloneDeep, isAllEmpty } from "@pureadmin/utils";
import {
  ascending,
  getTopMenu,
  initRouter,
  getHistoryMode,
  findRouteByPath,
  handleAliveRoute,
  formatTwoStageRoutes,
  formatFlatteningRoutes
} from "./utils";
import {
  type Router,
  type RouteRecordRaw,
  type RouteComponent,
  type RouteLocationRaw,
  type RouteLocationNormalized,
  createRouter
} from "vue-router";
import { ErrorCode, hasErrorCode } from "@/utils/http/errors";
import {
  LOGIN_PATH,
  FORCE_PASSWORD_PATH,
  SERVER_ERROR_PATH,
  HOME_PATH,
  WELCOME_PATH
} from "./paths";

/** 自动导入全部静态路由，无需再手动引入！匹配 src/router/modules 目录（任何嵌套级别）中具有 .ts 扩展名的所有文件，除了 remaining.ts 文件
 * 如何匹配所有文件请看：https://github.com/mrmlnc/fast-glob#basic-syntax
 * 如何排除文件请看：https://cn.vitejs.dev/guide/features.html#negative-patterns
 */
const modules: Record<string, any> = import.meta.glob(
  ["./modules/**/*.ts", "!./modules/**/remaining.ts"],
  {
    eager: true
  }
);

/** 原始静态路由（未做任何处理） */
const routes = [];

Object.keys(modules).forEach(key => {
  routes.push(modules[key].default);
});

/** 导出处理后的静态路由（三级及以上的路由全部拍成二级） */
export const constantRoutes: Array<RouteRecordRaw> = formatTwoStageRoutes(
  formatFlatteningRoutes(buildHierarchyTree(ascending(routes.flat(Infinity))))
);

/** 初始的静态路由，用于退出登录时重置路由 */
const initConstantRoutes: Array<RouteRecordRaw> = cloneDeep(constantRoutes);

/** 用于渲染菜单，保持原始层级 */
export const constantMenus: Array<RouteComponent> = ascending(
  routes.flat(Infinity)
).concat(...remainingRouter);

/** 不参与菜单的路由 */
export const remainingPaths = Object.keys(remainingRouter).map(v => {
  return remainingRouter[v].path;
});

/** 创建路由实例 */
export const router: Router = createRouter({
  history: getHistoryMode(import.meta.env.VITE_ROUTER_HISTORY),
  routes: constantRoutes.concat(...(remainingRouter as any)),
  strict: true,
  scrollBehavior(to, from, savedPosition) {
    return new Promise(resolve => {
      if (savedPosition) {
        return savedPosition;
      } else {
        if (from.meta.saveSrollTop) {
          const top: number =
            document.documentElement.scrollTop || document.body.scrollTop;
          resolve({ left: 0, top });
        }
      }
    });
  }
});

/** 记录已经加载的页面路径 */
const loadedPaths = new Set<string>();

/** 重置已加载页面记录 */
export function resetLoadedPaths() {
  loadedPaths.clear();
}

/** 重置路由 */
export function resetRouter() {
  router.clearRoutes();
  for (const route of initConstantRoutes.concat(...(remainingRouter as any))) {
    router.addRoute(route);
  }
  router.options.routes = formatTwoStageRoutes(
    formatFlatteningRoutes(buildHierarchyTree(ascending(routes.flat(Infinity))))
  );
  usePermissionStoreHook().clearAllCachePage();
  resetLoadedPaths();
}

/** 无需登录即可访问的页面 */
const publicPaths = new Set<string>([LOGIN_PATH, SERVER_ERROR_PATH]);

const { VITE_HIDE_HOME } = import.meta.env;

function setDocumentTitle(to: RouteLocationNormalized) {
  if (isUrl(to?.name as string)) return;
  const Title = getConfig().Title;
  const matched = to.matched.filter(item => item.meta?.title);
  const title = matched[matched.length - 1]?.meta.title as string | undefined;
  if (!title) return;
  document.title = Title ? `${title} | ${Title}` : title;
}

/** 刷新页面后把当前页加入标签页（与模板原逻辑一致） */
function restoreCurrentTag(path: string) {
  if (useMultiTagsStoreHook().getMultiTagsCache) return;
  const route = findRouteByPath(path, router.options.routes[0].children);
  getTopMenu(true);
  if (!route?.meta?.title) return;
  if (isAllEmpty(route.parentId) && route.meta?.backstage) {
    // 动态顶级路由（目录）
    const { path: childPath, name, meta } = route.children[0];
    useMultiTagsStoreHook().handleTags("push", { path: childPath, name, meta });
  } else {
    const { name, meta } = route;
    useMultiTagsStoreHook().handleTags("push", {
      path: route.path,
      name,
      meta
    });
  }
}

/** 会话探测或菜单加载失败时的去向：未登录去登录页，其他错误去 500 页 */
function failureTarget(err: unknown, to: RouteLocationNormalized) {
  if (hasErrorCode(err, ErrorCode.UNAUTHENTICATED)) {
    useUserStoreHook().clearSession();
    return { path: LOGIN_PATH, query: { redirect: to.fullPath } };
  }
  if (hasErrorCode(err, ErrorCode.PASSWORD_CHANGE_REQUIRED)) {
    useUserStoreHook().markMustChangePassword();
    return { path: FORCE_PASSWORD_PATH, query: { redirect: to.fullPath } };
  }
  console.error("[router] failed to restore session", err);
  return { path: SERVER_ERROR_PATH };
}

/**
 * 登录态守卫：
 * 1. 未加载用户信息时调用 GET /me 探测会话（HttpOnly Cookie，前端无法直接判断）；
 * 2. mustChangePassword 为 true 时只允许访问改密页；
 * 3. 首次进入时拉取 /me/menus 注册动态路由，未授权页面不会被注册，直接访问进入 404。
 */
async function authGuard(
  to: RouteLocationNormalized
): Promise<boolean | RouteLocationRaw> {
  const userStore = useUserStoreHook();
  const permissionStore = usePermissionStoreHook();

  // 已登录（本次页面会话内已加载 /me）时不再进入登录页
  if (to.path === LOGIN_PATH && userStore.isLoggedIn) return { path: "/" };
  if (publicPaths.has(to.path)) return true;

  if (!userStore.isLoggedIn) {
    try {
      await userStore.fetchMe();
    } catch (err) {
      return failureTarget(err, to);
    }
  }

  if (userStore.mustChangePassword) {
    return to.path === FORCE_PASSWORD_PATH
      ? true
      : { path: FORCE_PASSWORD_PATH, query: { redirect: to.fullPath } };
  }
  if (to.path === FORCE_PASSWORD_PATH) return { path: "/" };

  if (!permissionStore.routesLoaded) {
    try {
      await initRouter();
    } catch (err) {
      return failureTarget(err, to);
    }
    restoreCurrentTag(to.path);
    // 动态路由已注册，重新解析一次目标地址
    return { path: to.path, query: to.query, hash: to.hash, replace: true };
  }

  // name 为 URL 的是外链
  if (isUrl(to?.name as string)) {
    openLink(to.name as string);
    return false;
  }
  // 默认首页（「/」跳转到 VITE_HOME_PATH）不存在或无权限，及隐藏首页后手输 /welcome：
  // 改去工作台（未隐藏时）或第一个菜单
  const badHome =
    to.path === HOME_PATH &&
    HOME_PATH !== WELCOME_PATH &&
    to.name === "PageNotFound";
  const hiddenWelcome = VITE_HIDE_HOME === "true" && to.path === WELCOME_PATH;
  if (badHome || hiddenWelcome) {
    // 手输的 /welcome 先去默认首页（它不可用时再按 badHome 处理）
    const home =
      hiddenWelcome && HOME_PATH !== WELCOME_PATH ? HOME_PATH : fallbackHome();
    if (home && home !== to.path) return { path: home, replace: true };
  }
  return true;
}

/** 第一个可见的菜单页（不含工作台） */
function firstMenuPath(menus: RouteRecordRaw[]): string {
  for (const m of menus) {
    if (m.meta?.showLink === false) continue;
    if (m.children?.length) {
      const p = firstMenuPath(m.children);
      if (p) return p;
    } else if (m.path && m.path !== WELCOME_PATH && !isUrl(m.path)) {
      return m.path;
    }
  }
  return "";
}

/** 默认首页不可用时的去处：工作台（未隐藏时），否则第一个菜单 */
function fallbackHome(): string {
  if (VITE_HIDE_HOME !== "true") return WELCOME_PATH;
  return firstMenuPath(usePermissionStoreHook().wholeMenus as RouteRecordRaw[]);
}

router.beforeEach(async (to, from) => {
  to.meta.loaded = loadedPaths.has(to.path);
  if (!to.meta.loaded) NProgress.start();

  if (to.meta?.keepAlive) {
    handleAliveRoute(to as ToRouteType, "add");
    // 页面整体刷新和点击标签页刷新
    if (from.name === undefined || from.name === "Redirect") {
      handleAliveRoute(to as ToRouteType);
    }
  }
  setDocumentTitle(to);

  const result = await authGuard(to);
  if (result !== true) NProgress.done();
  return result;
});

router.afterEach(to => {
  loadedPaths.add(to.path);
  NProgress.done();
});

export default router;
