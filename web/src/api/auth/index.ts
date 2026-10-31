import { http, type PureHttpRequestConfig } from "@/utils/http";

export type MeUser = {
  id: string;
  username: string;
  nickname: string;
  email: string;
  phone: string;
  avatarUrl: string;
  deptId: string | null;
  deptName: string;
  version: number;
};

export type Me = {
  user: MeUser;
  roles: { id: string; code: string; name: string }[];
  /** 超级管理员会展开为全部权限码 */
  permissions: string[];
  isSuperAdmin: boolean;
  /** 为 true 时只能访问 /me、/me/password、/auth/logout */
  mustChangePassword: boolean;
};

/** /me/menus 返回的 pure-admin 动态路由 */
export type AsyncRoute = {
  /** 目录 "/system"，页面 "/system/user"，外链为完整 URL */
  path: string;
  name?: string;
  /** src/views 下不带扩展名的组件路径，如 "system/user/index" */
  component?: string;
  meta: {
    title: string;
    icon?: string;
    rank?: number;
    showLink?: boolean;
    keepAlive?: boolean;
    /** 目录下只有一个页面时仍显示目录（后端为目录的子菜单设置） */
    showParent?: boolean;
  };
  children?: AsyncRoute[];
};

export type LoginPayload = {
  username: string;
  password: string;
  captchaId?: string;
  captchaCode?: string;
};

export type Captcha = { id: string; image: string };
export type LoginOptions = { captchaAlways: boolean };

/** 登录失败时附带：下一次登录是否需要验证码 */
export type LoginFailureData = { captchaRequired?: boolean };
export type ChangePasswordPayload = {
  oldPassword: string;
  newPassword: string;
};
export type UpdateProfilePayload = {
  nickname: string;
  email: string;
  phone: string;
  avatarUrl: string;
  version: number;
};

export const login = (data: LoginPayload) => http.post<Me>("/auth/login", data);

/** 退出登录；会话可能已失效，所以不走全局 401 处理 */
export const logout = () =>
  http.post<null>("/auth/logout", undefined, { skipErrorHandler: true });

export const getMe = (config?: PureHttpRequestConfig) =>
  http.get<Me>("/me", undefined, config);

export const getMyMenus = (config?: PureHttpRequestConfig) =>
  http.get<AsyncRoute[]>("/me/menus", undefined, config);

export const changeMyPassword = (data: ChangePasswordPayload) =>
  http.put<null>("/me/password", data);

export const updateMyProfile = (data: UpdateProfilePayload) =>
  http.put<Me>("/me/profile", data);

export const getCaptcha = () =>
  http.get<Captcha>("/auth/captcha", undefined, { skipErrorHandler: true });

export const getLoginOptions = () =>
  http.get<LoginOptions>("/auth/login-options", undefined, {
    skipErrorHandler: true
  });
