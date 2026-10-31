import { request, type APIRequestContext } from "@playwright/test";
import { ADMIN_STATE, baseURL, type Credentials } from "./env";

type Envelope<T> = {
  code: number;
  message: string;
  data: T;
  requestId: string;
};

export type Me = {
  user: { id: string; username: string };
  permissions: string[];
  mustChangePassword: boolean;
};

export type AsyncRoute = {
  path: string;
  name?: string;
  meta: { title: string; showLink?: boolean };
  children?: AsyncRoute[];
};

export class ApiCallError extends Error {
  constructor(
    readonly status: number,
    readonly code: number,
    message: string
  ) {
    super(message);
  }
}

/**
 * 通过 Vite 代理直接调用后端（与浏览器同源），用于准备和清理测试数据。
 * 不安全方法会带上 Origin 和 X-XSRF-TOKEN，与浏览器行为一致。
 */
export class ApiClient {
  private constructor(private readonly ctx: APIRequestContext) {}

  static async create(): Promise<ApiClient> {
    const ctx = await request.newContext({
      baseURL,
      // 测试运行器会把配置中的 use.storageState（管理员会话）带进 newContext；
      // 必须显式清空，否则以其他账号登录时服务端会按防会话固定规则结束管理员会话
      storageState: { cookies: [], origins: [] },
      extraHTTPHeaders: { Origin: new URL(baseURL).origin }
    });
    return new ApiClient(ctx);
  }

  /** 复用全局初始化保存的管理员会话，不再重新登录 */
  static async admin(): Promise<ApiClient> {
    const ctx = await request.newContext({
      baseURL,
      storageState: ADMIN_STATE,
      extraHTTPHeaders: { Origin: new URL(baseURL).origin }
    });
    return new ApiClient(ctx);
  }

  static async login(creds: Credentials): Promise<ApiClient> {
    const client = await ApiClient.create();
    await client.call<Me>("POST", "/auth/login", creds);
    return client;
  }

  private async xsrfToken(): Promise<string | undefined> {
    const state = await this.ctx.storageState();
    return state.cookies.find(c => c.name === "XSRF-TOKEN")?.value;
  }

  async call<T>(method: string, path: string, data?: unknown): Promise<T> {
    const headers: Record<string, string> = {};
    if (method !== "GET") {
      const token = await this.xsrfToken();
      if (token) headers["X-XSRF-TOKEN"] = token;
    }
    const res = await this.ctx.fetch(`/api/v1${path}`, {
      method,
      headers,
      data: data === undefined ? undefined : data
    });
    const body = (await res.json().catch(() => null)) as Envelope<T> | null;
    if (!res.ok() || !body || body.code !== 0) {
      throw new ApiCallError(
        res.status(),
        body?.code ?? -1,
        `${method} ${path} -> HTTP ${res.status()} ${body?.code ?? ""} ${body?.message ?? ""}`
      );
    }
    return body.data;
  }

  me(): Promise<Me> {
    return this.call<Me>("GET", "/me");
  }

  menus(): Promise<AsyncRoute[]> {
    return this.call<AsyncRoute[]>("GET", "/me/menus");
  }

  async saveState(file: string): Promise<void> {
    await this.ctx.storageState({ path: file });
  }

  async dispose(): Promise<void> {
    await this.ctx.dispose();
  }
}

export type TempUser = Credentials & { id: string; roleId: string };

/**
 * 创建一个只拥有指定权限码的临时角色和用户。
 * 如果后端要求新用户首次登录改密，会自动改密并返回最终可用的密码。
 */
export async function createTempUser(
  admin: ApiClient,
  suffix: string,
  codes: string[]
): Promise<TempUser> {
  const role = await admin.call<{ id: string }>("POST", "/roles", {
    code: `e2e_${suffix}`
      .toLowerCase()
      .replace(/[^a-z0-9_]/g, "_")
      .slice(0, 32),
    name: `E2E ${suffix}`,
    dataScope: "self",
    deptIds: [],
    status: 1,
    sort: 999,
    remark: "created by e2e"
  });
  await admin.call("PUT", `/roles/${role.id}/permissions`, { codes });

  const username = `e2e_${suffix}`.slice(0, 32);
  let password = `E2e-${suffix}-Pw1`;
  const user = await admin.call<{ id: string }>("POST", "/users", {
    username,
    password,
    nickname: `E2E ${suffix}`,
    email: "",
    phone: "",
    deptId: null,
    status: 1,
    roleIds: [role.id],
    positionIds: []
  });

  const self = await ApiClient.login({ username, password });
  try {
    const me = await self.me();
    if (me.mustChangePassword) {
      const newPassword = `${password}-2`;
      await self.call("PUT", "/me/password", {
        oldPassword: password,
        newPassword
      });
      password = newPassword;
    }
    await self.call("POST", "/auth/logout");
  } finally {
    await self.dispose();
  }
  return { id: user.id, roleId: role.id, username, password };
}

/** 尽力清理临时数据，失败不影响测试结果 */
export async function removeTempUser(
  admin: ApiClient,
  user: TempUser | undefined
): Promise<void> {
  if (!user) return;
  await admin.call("DELETE", `/users/${user.id}`).catch(() => undefined);
  await admin.call("DELETE", `/roles/${user.roleId}`).catch(() => undefined);
}

/** 把菜单树展开为所有应显示在侧边栏的标题 */
export function visibleMenuTitles(routes: AsyncRoute[]): string[] {
  return routes.flatMap(r =>
    r.meta?.showLink === false
      ? []
      : [r.meta.title, ...visibleMenuTitles(r.children ?? [])]
  );
}
