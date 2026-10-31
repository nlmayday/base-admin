export default {
  path: "/account",
  redirect: "/account/index",
  meta: {
    icon: "ri/user-settings-line",
    title: "个人中心",
    showLink: false,
    rank: 99
  },
  children: [
    {
      path: "/account/index",
      name: "AccountCenter",
      component: () => import("@/views/account/index.vue"),
      meta: {
        title: "个人中心",
        showLink: false
      }
    }
  ]
} satisfies RouteConfigsTable;
