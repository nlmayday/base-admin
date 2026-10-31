<script setup lang="ts">
import { onMounted, ref, type Component } from "vue";
import { useRouter } from "vue-router";
import { hasPerm } from "@/utils/permission";
import { formatBytes } from "@/utils/format";
import { listUsers } from "@/api/iam/user";
import { listSessions } from "@/api/security";
import { getAuditStats } from "@/api/audit/stats";
import { getStorageStats } from "@/api/file";
import { listJobs } from "@/api/ops/job";
import UserIcon from "~icons/ri/user-3-line";
import ComputerIcon from "~icons/ri/computer-line";
import LoginIcon from "~icons/ri/login-circle-line";
import OperationIcon from "~icons/ri/file-list-3-line";
import FolderIcon from "~icons/ri/folder-3-line";
import TimerIcon from "~icons/ri/timer-line";

defineOptions({ name: "StatTiles" });

type TileValue = { value: string; sub: string };

type TileDef = {
  key: string;
  label: string;
  icon: Component;
  tone: "primary" | "success" | "warning" | "danger" | "info";
  /** 任一权限即可展示 */
  perms: string[];
  to: string;
  load: () => Promise<TileValue>;
};

type Tile = TileDef & { loading: boolean; data: TileValue | null };

const ONE = { page: 1, pageSize: 1 };

/** 审计统计被两个卡片共用，只请求一次 */
let auditStats: ReturnType<typeof getAuditStats> | null = null;
const audit = () => (auditStats ??= getAuditStats());

const DEFS: TileDef[] = [
  {
    key: "users",
    label: "用户总数",
    icon: UserIcon,
    tone: "primary",
    perms: ["iam:user:view"],
    to: "/system/user",
    load: async () => {
      const [all, enabled] = await Promise.all([
        listUsers(ONE),
        listUsers({ ...ONE, status: 1 })
      ]);
      return { value: String(all.total), sub: `启用 ${enabled.total}` };
    }
  },
  {
    key: "sessions",
    label: "在线会话",
    icon: ComputerIcon,
    tone: "success",
    perms: ["auth:session:view"],
    to: "/security/sessions",
    load: async () => {
      const page = await listSessions(ONE);
      return { value: String(page.total), sub: "当前有效登录" };
    }
  },
  {
    key: "logins",
    label: "今日登录",
    icon: LoginIcon,
    tone: "warning",
    perms: ["audit:login-log:view"],
    to: "/audit/login-log",
    load: async () => {
      const { today } = await audit();
      return {
        value: String(today.loginSuccess),
        sub: `失败 ${today.loginFailure} 次`
      };
    }
  },
  {
    key: "operations",
    label: "今日操作",
    icon: OperationIcon,
    tone: "info",
    perms: ["audit:operation-log:view"],
    to: "/audit/operation-log",
    load: async () => {
      const { today } = await audit();
      return {
        value: String(today.operations),
        sub: `失败 ${today.operationFailures} 次`
      };
    }
  },
  {
    key: "files",
    label: "文件存储",
    icon: FolderIcon,
    tone: "primary",
    perms: ["file:storage:view"],
    to: "/file/storage",
    load: async () => {
      const s = await getStorageStats();
      return {
        value: formatBytes(s.totalSize),
        sub: `${s.fileCount} 个文件`
      };
    }
  },
  {
    key: "jobs",
    label: "定时任务",
    icon: TimerIcon,
    tone: "danger",
    perms: ["ops:job:view"],
    to: "/ops/jobs",
    load: async () => {
      const [all, enabled] = await Promise.all([
        listJobs(ONE),
        listJobs({ ...ONE, enabled: "true" })
      ]);
      return { value: String(all.total), sub: `运行中 ${enabled.total}` };
    }
  }
];

const router = useRouter();
const tiles = ref<Tile[]>(
  DEFS.filter(d => d.perms.some(p => hasPerm(p))).map(d => ({
    ...d,
    loading: true,
    data: null
  }))
);

function open(tile: Tile): void {
  if (router.getRoutes().some(r => r.path === tile.to)) router.push(tile.to);
}

async function loadTile(index: number): Promise<void> {
  const tile = tiles.value[index];
  try {
    const data = await tile.load();
    tiles.value[index] = { ...tile, loading: false, data };
  } catch {
    tiles.value[index] = { ...tile, loading: false, data: null };
  }
}

function reload(): Promise<void[]> {
  auditStats = null;
  tiles.value = tiles.value.map(t => ({ ...t, loading: true }));
  return Promise.all(tiles.value.map((_, i) => loadTile(i)));
}

onMounted(reload);
defineExpose({ reload, count: () => tiles.value.length });
</script>

<template>
  <div v-if="tiles.length" class="tiles">
    <div
      v-for="tile in tiles"
      :key="tile.key"
      class="tile"
      :class="`tile--${tile.tone}`"
      role="link"
      tabindex="0"
      @click="open(tile)"
      @keyup.enter="open(tile)"
    >
      <div class="tile-icon">
        <component :is="tile.icon" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-sm text-[var(--el-text-color-secondary)]">
          {{ tile.label }}
        </div>
        <el-skeleton v-if="tile.loading" :rows="0" animated class="mt-2 w-20" />
        <template v-else>
          <div class="mt-1 truncate text-2xl font-semibold tabular-nums">
            {{ tile.data?.value ?? "-" }}
          </div>
          <div class="text-xs text-[var(--el-text-color-secondary)]">
            {{ tile.data?.sub ?? "加载失败" }}
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.tile {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 16px 18px;
  cursor: pointer;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  transition:
    box-shadow 0.2s,
    transform 0.2s;
}

.tile:hover,
.tile:focus-visible {
  outline: none;
  box-shadow: var(--el-box-shadow-light);
  transform: translateY(-2px);
}

.tile-icon {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  font-size: 22px;
  color: var(--tile-color);
  background: var(--tile-bg);
  border-radius: 10px;
}

.tile--primary {
  --tile-color: var(--el-color-primary);
  --tile-bg: var(--el-color-primary-light-9);
}

.tile--success {
  --tile-color: var(--el-color-success);
  --tile-bg: var(--el-color-success-light-9);
}

.tile--warning {
  --tile-color: var(--el-color-warning);
  --tile-bg: var(--el-color-warning-light-9);
}

.tile--danger {
  --tile-color: var(--el-color-danger);
  --tile-bg: var(--el-color-danger-light-9);
}

.tile--info {
  --tile-color: var(--el-color-info);
  --tile-bg: var(--el-color-info-light-9);
}
</style>
