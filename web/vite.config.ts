import { getPluginsList } from "./build/plugins";
import { include, exclude } from "./build/optimize";
import { type UserConfigExport, type ConfigEnv, loadEnv } from "vite";
import {
  root,
  alias,
  wrapperEnv,
  pathResolve,
  __APP_INFO__
} from "./build/utils";

export default ({ mode, command }: ConfigEnv): UserConfigExport => {
  const env = loadEnv(mode, root);
  const { VITE_CDN, VITE_PORT, VITE_COMPRESSION, VITE_PUBLIC_PATH } =
    wrapperEnv(env);
  // 后端地址，默认 http://127.0.0.1:8080；可通过 VITE_API_PROXY_TARGET 覆盖
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://127.0.0.1:8080";
  return {
    base: VITE_PUBLIC_PATH,
    root,
    resolve: {
      alias: [
        ...Object.entries(alias).map(([find, replacement]) => ({
          find,
          replacement
        })),
        // 生产构建时 element-plus 主入口会把全部组件打包进来，改为按组件子路径导入
        // （CDN 模式下 element-plus 整包来自 CDN，不需要）
        ...(command === "build" && !VITE_CDN
          ? [
              {
                find: /^element-plus$/,
                replacement: pathResolve(
                  "./src/plugins/element-plus-entry.ts",
                  import.meta.url
                )
              }
            ]
          : [])
      ]
    },
    // 服务端渲染
    server: {
      // 端口号
      port: VITE_PORT,
      host: "0.0.0.0",
      // 本地跨域代理 https://cn.vitejs.dev/config/server-options.html#server-proxy
      // 开发期同源：/api 代理到后端，Cookie 与 XSRF 校验都按同源工作。
      // 不改写 Origin，后端需要把 http://localhost:<VITE_PORT> 加入允许列表。
      proxy: {
        "/api": {
          target: apiProxyTarget,
          changeOrigin: false
        }
      },
      // 预热文件以提前转换和缓存结果，降低启动期间的初始页面加载时长并防止转换瀑布
      warmup: {
        clientFiles: ["./index.html", "./src/{views,components}/*"]
      }
    },
    plugins: getPluginsList(VITE_CDN, VITE_COMPRESSION),
    // https://cn.vitejs.dev/config/dep-optimization-options.html#dep-optimization-options
    optimizeDeps: {
      include,
      exclude
    },
    build: {
      // https://cn.vitejs.dev/guide/build.html#browser-compatibility
      target: "es2015",
      sourcemap: false,
      // 单个 chunk 超过 800 KB（压缩前）时告警，防止体积回退
      chunkSizeWarningLimit: 800,
      rollupOptions: {
        input: {
          index: pathResolve("./index.html", import.meta.url)
        },
        // 静态资源分类打包
        output: {
          chunkFileNames: "static/js/[name]-[hash].js",
          entryFileNames: "static/js/[name]-[hash].js",
          assetFileNames: "static/[ext]/[name]-[hash].[ext]",
          // 体积大、很少变化的依赖单独成块：业务代码更新时浏览器缓存仍然有效
          manualChunks(id) {
            if (!id.includes("node_modules")) return;
            if (/[\\/](echarts|zrender)[\\/]/.test(id)) return "echarts";
            if (/[\\/](element-plus|@element-plus)[\\/]/.test(id)) {
              return "element-plus";
            }
            if (/[\\/](@vue|vue|vue-router|pinia)[\\/]/.test(id)) return "vue";
          }
        }
      }
    },
    define: {
      __INTLIFY_PROD_DEVTOOLS__: false,
      __APP_INFO__: JSON.stringify(__APP_INFO__)
    }
  };
};
