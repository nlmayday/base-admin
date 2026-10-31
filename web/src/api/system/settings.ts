import { http } from "@/utils/http";

export type Settings = {
  siteName: string;
  logoUrl: string;
  uploadAllowedExtensions: string[];
  uploadMaxSizeMB: number;
  auditRetentionDays: number;
};

/** 版本号是各设置项版本之和，提交时原样带回 */
export type SettingsView = Settings & { version: number };

export type PublicSettings = Pick<Settings, "siteName" | "logoUrl">;

export const getSettings = () => http.get<SettingsView>("/settings");

export const updateSettings = (data: Settings & { version: number }) =>
  http.put<SettingsView>("/settings", data);

/** 无需登录（登录页等使用） */
export const getPublicSettings = () =>
  http.get<PublicSettings>("/settings/public", undefined, {
    skipErrorHandler: true
  });
