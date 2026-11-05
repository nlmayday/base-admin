import { HOME_PATH } from "../paths";

const { VITE_HIDE_HOME } = import.meta.env;
const Layout = () => import("@/layout/index.vue");

export default {
  path: "/",
  name: "Home",
  component: Layout,
  // 默认首页（VITE_HOME_PATH）；不存在或无权限时由路由守卫改去工作台或第一个菜单
  redirect: HOME_PATH,
  meta: {
    icon: "ri/dashboard-line",
    title: "工作台",
    rank: 0
  },
  children: [
    {
      path: "/welcome",
      name: "Welcome",
      component: () => import("@/views/welcome/index.vue"),
      meta: {
        title: "工作台",
        showLink: VITE_HIDE_HOME === "true" ? false : true
      }
    }
  ]
} satisfies RouteConfigsTable;
