import { mkdir } from "node:fs/promises";
import path from "node:path";
import { ApiClient } from "./api";
import { ADMIN_STATE, adminCredentials, type Credentials } from "./env";

/**
 * 管理员会话。全新环境（如 CI）里管理员还在用初始密码：设置了
 * E2E_BOOTSTRAP_PASSWORD 时，先用它登录并改成 E2E_ADMIN_PASSWORD。
 */
async function adminSession(creds: Credentials): Promise<ApiClient> {
  const bootstrap = process.env.E2E_BOOTSTRAP_PASSWORD;
  try {
    return await ApiClient.login(creds);
  } catch (err) {
    if (!bootstrap) throw err;
  }
  const first = await ApiClient.login({ ...creds, password: bootstrap });
  try {
    await first.call("PUT", "/me/password", {
      oldPassword: bootstrap,
      newPassword: creds.password
    });
  } finally {
    await first.dispose();
  }
  return ApiClient.login(creds);
}

/** 登录一次管理员并保存会话，供所有用例复用 */
export default async function globalSetup(): Promise<void> {
  const admin = await adminSession(adminCredentials());
  try {
    const me = await admin.me();
    if (me.mustChangePassword) {
      throw new Error(
        "E2E 管理员账号需要先完成首次改密（或设置 E2E_BOOTSTRAP_PASSWORD）"
      );
    }
    await mkdir(path.dirname(ADMIN_STATE), { recursive: true });
    await admin.saveState(ADMIN_STATE);
  } finally {
    await admin.dispose();
  }
}
