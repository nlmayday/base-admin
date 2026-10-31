import {
  type RouterHistory,
  type RouteRecordRaw,
  type RouteComponent,
  createWebHistory,
  createWebHashHistory
} from "vue-router";
import { router } from "./index";
import { isProxy, toRaw } from "vue";
import { useTimeoutFn } from "@vueuse/core";
import { isUrl, cloneDeep, isAllEmpty, intersection } from "@pureadmin/utils";
import { buildHierarchyTree } from "@/utils/tree";
import { type menuType, routerArrays } from "@/layout/types";
import { useMultiTagsStoreHook } from "@/store/modules/multiTags";
import { usePermissionStoreHook } from "@/store/modules/permission";
// https://cn.vitejs.dev/guide/features.html#glob-import
const modulesRoutes = import.meta.glob("/src/views/**/*.{vue,tsx}");
const NotFoundView = () => import("@/views/error/404.vue");

// 动态路由
import { type AsyncRoute, getMyMenus } from "@/api/auth";

function handRank(routeInfo: any) {
  const { name, path, parentId, meta } = routeInfo;
  return isAllEmpty(parentId)
    ? isAllEmpty(meta?.rank) ||
      (meta?.rank === 0 && name !== "Home" && path !== "/")
      ? true
      : false
    : false;
}

/** 按照路由中meta下的rank等级升序来排序路由 */
function ascending(arr: any[]) {
  arr.forEach((v, index) => {
    // 当rank不存在时，根据顺序自动创建，首页路由永远在第一位
    if (handRank(v)) v.meta.rank = index + 2;
  });
  return arr.sort(
    (a: { meta: { rank: number } }, b: { meta: { rank: number } }) => {
      return a?.meta.rank - b?.meta.rank;
    }
  );
}

/** 过滤meta中showLink为false的菜单 */
function filterTree(data: RouteComponent[]) {
  const newTree = cloneDeep(data).filter(
    (v: { meta: { showLink: boolean } }) => v.meta?.showLink !== false
  );
  newTree.forEach(
    (v: { children }) => v.children && (v.children = filterTree(v.children))
  );
  return newTree;
}

/** 过滤children长度为0的的目录，当目录下没有菜单时，会过滤此目录，目录没有赋予roles权限，当目录下只要有一个菜单有显示权限，那么此目录就会显示 */
function filterChildrenTree(data: RouteComponent[]) {
  const newTree = cloneDeep(data).filter((v: any) => v?.children?.length !== 0);
  newTree.forEach(
    (v: { children }) => v.children && (v.children = filterTree(v.children))
  );
  return newTree;
}

/** 判断两个数组彼此是否存在相同值 */
function isOneOfArray(a: Array<string>, b: Array<string>) {
  return Array.isArray(a) && Array.isArray(b)
    ? intersection(a, b).length > 0
      ? true
      : false
    : true;
}

/**
 * 过滤空目录。菜单可见性完全由后端 /me/menus 决定（只返回有 view 权限的菜单），
 * 前端不再按角色二次过滤。
 */
function filterNoPermissionTree(data: RouteComponent[]) {
  return filterChildrenTree(data);
}

/** 通过指定 `key` 获取父级路径集合，默认 `key` 为 `path` */
function getParentPaths(value: string, routes: RouteRecordRaw[], key = "path") {
  // 深度遍历查找
  function dfs(routes: RouteRecordRaw[], value: string, parents: string[]) {
    for (let i = 0; i < routes.length; i++) {
      const item = routes[i];
      // 返回父级path
      if (item[key] === value) return parents;
      // children不存在或为空则不递归
      if (!item.children || !item.children.length) continue;
      // 往下查找时将当前path入栈
      parents.push(item.path);

      if (dfs(item.children, value, parents).length) return parents;
      // 深度遍历查找未找到时当前path 出栈
      parents.pop();
    }
    // 未找到时返回空数组
    return [];
  }

  return dfs(routes, value, []);
}

/** 查找对应 `path` 的路由信息 */
function findRouteByPath(path: string, routes: RouteRecordRaw[]) {
  let res = routes.find((item: { path: string }) => item.path == path);
  if (res) {
    return isProxy(res) ? toRaw(res) : res;
  } else {
    for (let i = 0; i < routes.length; i++) {
      if (
        routes[i].children instanceof Array &&
        routes[i].children.length > 0
      ) {
        res = findRouteByPath(path, routes[i].children);
        if (res) {
          return isProxy(res) ? toRaw(res) : res;
        }
      }
    }
    return null;
  }
}

/** 动态路由注册完成后，再添加全屏404（页面不存在）页面，避免刷新动态路由页面时误跳转到404页面 */
function addPathMatch() {
  if (!router.hasRoute("pathMatch")) {
    router.addRoute({
      path: "/:pathMatch(.*)*",
      name: "PageNotFound",
      component: () => import("@/views/error/404.vue"),
      meta: {
        title: "404",
        showLink: false
      }
    });
  }
}

/** 处理动态路由（后端返回的路由） */
function handleAsyncRoutes(asyncRoutes: AsyncRoute[]) {
  const routeList = addAsyncRoutes(asyncRoutes);
  if (routeList.length === 0) {
    usePermissionStoreHook().handleWholeMenus(routeList);
  } else {
    // 注册时去掉 children：所有页面都已拍平为 "/" 的子路由，目录只保留 redirect
    formatFlatteningRoutes(cloneDeep(routeList)).map(
      ({ children: _children, ...rest }: RouteRecordRaw) => {
        const v = rest as RouteRecordRaw;
        // 防止重复添加路由
        if (
          router.options.routes[0].children.findIndex(
            value => value.path === v.path
          ) !== -1
        ) {
          return;
        } else {
          // 切记将路由push到routes后还需要使用addRoute，这样路由才能正常跳转
          router.options.routes[0].children.push(v);
          // 最终路由进行升序
          ascending(router.options.routes[0].children);
          if (!router.hasRoute(v?.name)) router.addRoute(v);
          const flattenRouters: any = router
            .getRoutes()
            .find(n => n.path === "/");
          // 保持router.options.routes[0].children与path为"/"的children一致，防止数据不一致导致异常
          flattenRouters.children = router.options.routes[0].children;
          router.addRoute(flattenRouters);
        }
      }
    );
    usePermissionStoreHook().handleWholeMenus(routeList);
  }
  if (!useMultiTagsStoreHook().getMultiTagsCache) {
    useMultiTagsStoreHook().handleTags("equal", [
      ...routerArrays,
      ...usePermissionStoreHook().flatteningRoutes.filter(
        v => v?.meta?.fixedTag
      )
    ]);
  }
  addPathMatch();
}

/**
 * 初始化路由：每次登录或刷新页面都从 GET /me/menus 重新拉取，
 * 不在 localStorage 缓存（菜单必须反映后端当前的授权）。
 */
async function initRouter() {
  const menus = await getMyMenus({ skipErrorHandler: true });
  handleAsyncRoutes(Array.isArray(menus) ? menus : []);
  usePermissionStoreHook().setRoutesLoaded(true);
  return router;
}

/** 外链菜单注册的路由前缀，侧边栏据此识别外链 */
export const EXTERNAL_PATH_PREFIX = "/external/";

/**
 * 菜单或权限变更后重新拉取 /me/menus，原地刷新侧边栏并注册新增的页面，
 * 不需要刷新浏览器。已移除的页面路由保留到下次刷新，访问时由后端鉴权拒绝。
 */
async function refreshMenus(): Promise<void> {
  const menus = await getMyMenus({ skipErrorHandler: true });
  handleAsyncRoutes(Array.isArray(menus) ? menus : []);
}

/**
 * 将多级嵌套路由处理成一维数组
 * @param routesList 传入路由
 * @returns 返回处理后的一维路由
 */
function formatFlatteningRoutes(routesList: RouteRecordRaw[]) {
  if (routesList.length === 0) return routesList;
  let hierarchyList = buildHierarchyTree(routesList);
  for (let i = 0; i < hierarchyList.length; i++) {
    if (hierarchyList[i].children) {
      hierarchyList = hierarchyList
        .slice(0, i + 1)
        .concat(hierarchyList[i].children, hierarchyList.slice(i + 1));
    }
  }
  return hierarchyList;
}

/**
 * 一维数组处理成多级嵌套数组（三级及以上的路由全部拍成二级，keep-alive 只支持到二级缓存）
 * https://github.com/pure-admin/vue-pure-admin/issues/67
 * @param routesList 处理后的一维路由菜单数组
 * @returns 返回将一维数组重新处理成规定路由的格式
 */
function formatTwoStageRoutes(routesList: RouteRecordRaw[]) {
  if (routesList.length === 0) return routesList;
  const newRoutesList: RouteRecordRaw[] = [];
  routesList.forEach((v: RouteRecordRaw) => {
    if (v.path === "/") {
      newRoutesList.push({
        component: v.component,
        name: v.name,
        path: v.path,
        redirect: v.redirect,
        meta: v.meta,
        children: []
      });
    } else {
      newRoutesList[0]?.children.push({ ...v });
    }
  });
  return newRoutesList;
}

/** 处理缓存路由（添加、删除、刷新） */
function handleAliveRoute({ name }: ToRouteType, mode?: string) {
  switch (mode) {
    case "add":
      usePermissionStoreHook().cacheOperate({
        mode: "add",
        name
      });
      break;
    case "delete":
      usePermissionStoreHook().cacheOperate({
        mode: "delete",
        name
      });
      break;
    case "refresh":
      usePermissionStoreHook().cacheOperate({
        mode: "refresh",
        name
      });
      break;
    default:
      usePermissionStoreHook().cacheOperate({
        mode: "delete",
        name
      });
      useTimeoutFn(() => {
        usePermissionStoreHook().cacheOperate({
          mode: "add",
          name
        });
      }, 100);
  }
}

/** 按后端给出的 component（src/views 下不带扩展名的路径）精确查找页面组件 */
function resolveView(component: string) {
  const clean = component.replace(/^\/+/, "").replace(/\.(vue|tsx)$/, "");
  return (
    modulesRoutes[`/src/views/${clean}.vue`] ??
    modulesRoutes[`/src/views/${clean}.tsx`]
  );
}

/** 简单稳定的字符串哈希，用于给外链生成站内占位路径 */
function hashString(value: string): string {
  let hash = 0;
  for (let i = 0; i < value.length; i++) {
    hash = (hash * 31 + value.charCodeAt(i)) | 0;
  }
  return Math.abs(hash).toString(36);
}

/**
 * 外链：契约中 path 为完整 URL；pure-admin 约定外链的 name 为 URL、path 为站内占位路径，
 * 侧边栏据此渲染成 <a target="_blank">。
 */
function toExternalRoute(route: AsyncRoute): RouteRecordRaw {
  const slug = route.name ? route.name.toLowerCase() : hashString(route.path);
  return {
    path: `${EXTERNAL_PATH_PREFIX}${slug}`,
    name: route.path,
    component: NotFoundView,
    meta: { ...route.meta, backstage: true }
  } as RouteRecordRaw;
}

/** 把 /me/menus 返回的路由转换成 vue-router 路由（不修改入参） */
function addAsyncRoutes(arrRoutes: AsyncRoute[]): RouteRecordRaw[] {
  if (!arrRoutes || !arrRoutes.length) return [];
  return arrRoutes.map((v): RouteRecordRaw => {
    if (isUrl(v.path)) return toExternalRoute(v);
    const children = v.children?.length ? addAsyncRoutes(v.children) : [];
    const record: Record<string, unknown> = {
      path: v.path,
      meta: { ...v.meta, backstage: true }
    };
    if (children.length) {
      record.children = children;
      // 目录默认重定向到第一个子菜单；目录 name 缺省时用第一个子菜单的 name 加 Parent，避免重名
      record.redirect = children[0].path;
      record.name = v.name ?? `${String(children[0].name)}Parent`;
    } else if (v.name) {
      record.name = v.name;
    }
    if (v.component) {
      const view = resolveView(v.component);
      if (!view) {
        console.warn(
          `[router] view "${v.component}" for "${v.path}" not found under src/views`
        );
      }
      record.component = view ?? NotFoundView;
    } else if (!children.length) {
      record.component = NotFoundView;
    }
    return record as unknown as RouteRecordRaw;
  });
}

/** 获取路由历史模式 https://next.router.vuejs.org/zh/guide/essentials/history-mode.html */
function getHistoryMode(routerHistory): RouterHistory {
  // len为1 代表只有历史模式 为2 代表历史模式中存在base参数 https://next.router.vuejs.org/zh/api/#%E5%8F%82%E6%95%B0-1
  const historyMode = routerHistory.split(",");
  const leftMode = historyMode[0];
  const rightMode = historyMode[1];
  // no param
  if (historyMode.length === 1) {
    if (leftMode === "hash") {
      return createWebHashHistory("");
    } else if (leftMode === "h5") {
      return createWebHistory("");
    }
  } //has param
  else if (historyMode.length === 2) {
    if (leftMode === "hash") {
      return createWebHashHistory(rightMode);
    } else if (leftMode === "h5") {
      return createWebHistory(rightMode);
    }
  }
}

function handleTopMenu(route) {
  if (route?.children && route.children.length > 1) {
    if (route.redirect) {
      return route.children.filter(cur => cur.path === route.redirect)[0];
    } else {
      return route.children[0];
    }
  } else {
    return route;
  }
}

/** 获取所有菜单中的第一个菜单（顶级菜单）*/
function getTopMenu(tag = false): menuType {
  const topMenu = handleTopMenu(
    usePermissionStoreHook().wholeMenus[0]?.children[0]
  );
  tag && useMultiTagsStoreHook().handleTags("push", topMenu);
  return topMenu;
}

export {
  ascending,
  filterTree,
  initRouter,
  refreshMenus,
  getTopMenu,
  addPathMatch,
  isOneOfArray,
  getHistoryMode,
  addAsyncRoutes,
  getParentPaths,
  findRouteByPath,
  handleAliveRoute,
  formatTwoStageRoutes,
  formatFlatteningRoutes,
  filterNoPermissionTree
};
