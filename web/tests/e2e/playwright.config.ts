import { defineConfig, devices } from "@playwright/test";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { ADMIN_STATE } from "./support/env";

/**
 * E2E 需要真实后端：Vite dev server（默认 http://localhost:8848）把 /api 代理到后端。
 * 运行前设置 E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD（一个已改过初始密码的超级管理员）。
 */
const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:8848";
const webRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../.."
);

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: "playwright-report" }]
  ],
  outputDir: "test-results",
  globalSetup: "./support/global-setup.ts",
  use: {
    baseURL,
    storageState: ADMIN_STATE,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    locale: "zh-CN"
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  // 未设置 E2E_BASE_URL 时自动启动（或复用已启动的）Vite dev server
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: "pnpm dev",
        cwd: webRoot,
        url: baseURL,
        reuseExistingServer: true,
        timeout: 120_000
      }
});
