import path from "node:path";
import { fileURLToPath } from "node:url";

function required(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(
      `环境变量 ${name} 未设置。E2E 需要一个已完成首次改密的超级管理员账号。`
    );
  }
  return value;
}

export type Credentials = { username: string; password: string };

export function adminCredentials(): Credentials {
  return {
    username: required("E2E_ADMIN_USERNAME"),
    password: required("E2E_ADMIN_PASSWORD")
  };
}

export const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:8848";

/**
 * 全局初始化时保存的管理员会话（见 global-setup.ts）。
 * 所有用例默认以该会话打开页面，避免每个用例都登录而触发登录限流。
 */
export const ADMIN_STATE = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../.auth/admin.json"
);

/** 测试中创建的临时数据统一前缀，便于排查与清理 */
export function uniqueSuffix(): string {
  return `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4)}`;
}
