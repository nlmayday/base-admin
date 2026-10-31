import { defineStore } from "pinia";
import {
  store,
  router,
  resetRouter,
  routerArrays,
  storageLocal
} from "../utils";
import {
  type Me,
  type LoginPayload,
  login as loginApi,
  logout as logoutApi,
  getMe
} from "@/api/auth";
import { useMultiTagsStoreHook } from "./multiTags";
import { LOGIN_PATH } from "@/router/paths";

/** localStorage 中只保存非敏感的展示信息，不保存任何凭据或权限 */
const DISPLAY_KEY = "user-display";

type DisplayInfo = { nickname: string; avatar: string };

type UserState = {
  me: Me | null;
  /** 权限码集合，用于 hasPerm 快速判断 */
  permissions: ReadonlySet<string>;
  username: string;
  nickname: string;
  avatar: string;
};

function readDisplay(): DisplayInfo {
  const info = storageLocal().getItem<Partial<DisplayInfo>>(DISPLAY_KEY);
  return { nickname: info?.nickname ?? "", avatar: info?.avatar ?? "" };
}

export const useUserStore = defineStore("pure-user", {
  state: (): UserState => ({
    me: null,
    permissions: new Set<string>(),
    username: "",
    ...readDisplay()
  }),
  getters: {
    isLoggedIn: (state): boolean => state.me !== null,
    mustChangePassword: (state): boolean =>
      state.me?.mustChangePassword ?? false,
    isSuperAdmin: (state): boolean => state.me?.isSuperAdmin ?? false
  },
  actions: {
    /** 写入 /me 的结果 */
    setMe(me: Me) {
      this.me = me;
      this.permissions = new Set(me.permissions ?? []);
      this.username = me.user.username;
      this.nickname = me.user.nickname;
      this.avatar = me.user.avatarUrl;
      storageLocal().setItem<DisplayInfo>(DISPLAY_KEY, {
        nickname: me.user.nickname,
        avatar: me.user.avatarUrl
      });
    },
    /** POST /auth/login，成功后后端已设置会话 Cookie */
    async login(payload: LoginPayload): Promise<Me> {
      const me = await loginApi(payload);
      this.setMe(me);
      return me;
    },
    /** GET /me；用于刷新页面后恢复状态（会话探测，不弹全局提示） */
    async fetchMe(): Promise<Me> {
      const me = await getMe({ skipErrorHandler: true });
      this.setMe(me);
      return me;
    },
    /** 收到 20005 时标记需要改密 */
    markMustChangePassword() {
      if (this.me && !this.me.mustChangePassword) {
        this.me = { ...this.me, mustChangePassword: true };
      }
    },
    /** 清空本地用户状态与动态路由（不调用接口） */
    clearSession() {
      this.me = null;
      this.permissions = new Set<string>();
      this.username = "";
      this.nickname = "";
      this.avatar = "";
      storageLocal().removeItem(DISPLAY_KEY);
      useMultiTagsStoreHook().handleTags("equal", [...routerArrays]);
      resetRouter();
    },
    /** 退出登录：POST /auth/logout，无论成功与否都清空本地状态 */
    async logout() {
      try {
        await logoutApi();
      } catch (err) {
        // 会话可能已失效，仍然继续清理本地状态
        console.warn("[logout] server logout failed", err);
      } finally {
        this.clearSession();
        await router.replace(LOGIN_PATH);
      }
    }
  }
});

export function useUserStoreHook() {
  return useUserStore(store);
}
