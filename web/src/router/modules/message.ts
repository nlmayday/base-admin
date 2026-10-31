export default {
  path: "/message",
  redirect: "/message/index",
  meta: {
    icon: "ri/notification-3-line",
    title: "我的消息",
    showLink: false,
    rank: 98
  },
  children: [
    {
      path: "/message/index",
      name: "MyMessages",
      component: () => import("@/views/message/index.vue"),
      meta: {
        title: "我的消息",
        showLink: false
      }
    }
  ]
} satisfies RouteConfigsTable;
