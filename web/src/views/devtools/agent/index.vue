<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useClipboard } from "@vueuse/core";
import { message } from "@/utils/message";
import { formatDateTime } from "@/utils/format";
import { confirmAction } from "@/composables/useConfirmAction";
import {
  listAgentKeys,
  revokeAgentKey,
  type AgentKey,
  type AgentKeyList
} from "@/api/devtools/agent";
import CreateKeyDialog from "./components/CreateKeyDialog.vue";
import { agentBaseUrl, agentPrompt, keyState, type KeyState } from "./prompt";

defineOptions({ name: "DevtoolsAgent" });

const STATE_TAGS: Record<
  KeyState,
  { label: string; type: "success" | "info" | "danger" }
> = {
  active: { label: "可用", type: "success" },
  expired: { label: "已过期", type: "info" },
  revoked: { label: "已撤销", type: "danger" }
};

const loading = ref(false);
const data = ref<AgentKeyList>({ enabled: true, endpoint: "", keys: [] });
const dialogVisible = ref(false);
const { copy } = useClipboard({ legacy: true });

const baseUrl = computed(() =>
  data.value.endpoint
    ? agentBaseUrl(window.location.origin, data.value.endpoint)
    : ""
);

async function load(): Promise<void> {
  loading.value = true;
  try {
    data.value = await listAgentKeys();
  } finally {
    loading.value = false;
  }
}

async function copyText(text: string, done: string): Promise<void> {
  try {
    await copy(text);
    message(done, { type: "success" });
  } catch {
    message("复制失败，请手动选择文本复制", { type: "error" });
  }
}

function revoke(row: AgentKey): void {
  confirmAction({
    confirm: `撤销密钥「${row.name}」（${row.prefix}…）后，使用它的 AI 将无法再调用接口，确定撤销？`,
    success: "密钥已撤销",
    action: () => revokeAgentKey(row.id),
    reload: load
  });
}

onMounted(load);
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3">
      <template #header>
        <span class="card-title">AI 接口</span>
      </template>
      <el-alert
        v-if="!data.enabled"
        type="warning"
        :closable="false"
        show-icon
        class="mb-3"
        title="当前不是开发环境（app.env ≠ dev），AI 接口已关闭；密钥只能在本地开发环境使用。"
      />
      <div class="text-sm leading-7 text-[var(--el-text-color-regular)]">
        <p>
          给 AI 编程助手（Claude Code、Cursor
          等）用的接口：按框架约定建业务表，并一键生成增删改查（后端接口、前端页面、菜单、权限）。
          AI 用密钥调用，操作记入操作日志，身份是密钥的创建人。
        </p>
        <p>
          用法：创建密钥 → 复制「给 AI 的说明」发给 AI → AI
          读取接口说明后自行建表、生成模块 → 重启后端。
        </p>
      </div>
      <div v-if="baseUrl" class="mt-2 flex flex-wrap items-center gap-2">
        <span class="text-sm text-[var(--el-text-color-secondary)]">
          接口地址
        </span>
        <el-tag type="info" disable-transitions>{{ baseUrl }}</el-tag>
        <el-button link type="primary" @click="copyText(baseUrl, '地址已复制')">
          复制地址
        </el-button>
        <el-button
          link
          type="primary"
          @click="
            copyText(
              agentPrompt(baseUrl),
              '说明已复制，记得把 <密钥> 换成真实密钥'
            )
          "
        >
          复制给 AI 的说明
        </el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="card-title">密钥</span>
          <div>
            <el-button @click="load">刷新</el-button>
            <el-button type="primary" @click="dialogVisible = true">
              创建密钥
            </el-button>
          </div>
        </div>
      </template>
      <el-table v-loading="loading" :data="data.keys" row-key="id">
        <el-table-column prop="name" label="用途" min-width="160" />
        <el-table-column label="密钥" min-width="150">
          <template #default="{ row }">
            <code>{{ (row as AgentKey).prefix }}…</code>
          </template>
        </el-table-column>
        <el-table-column prop="userName" label="创建人" min-width="100" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag
              :type="STATE_TAGS[keyState(row as AgentKey)].type"
              disable-transitions
            >
              {{ STATE_TAGS[keyState(row as AgentKey)].label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最近使用" min-width="160">
          <template #default="{ row }">
            {{
              (row as AgentKey).lastUsedAt
                ? formatDateTime((row as AgentKey).lastUsedAt)
                : "未使用"
            }}
          </template>
        </el-table-column>
        <el-table-column label="过期时间" min-width="160">
          <template #default="{ row }">
            {{
              (row as AgentKey).expiresAt
                ? formatDateTime((row as AgentKey).expiresAt)
                : "永不过期"
            }}
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="160">
          <template #default="{ row }">
            {{ formatDateTime((row as AgentKey).createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="keyState(row as AgentKey) === 'active'"
              link
              type="danger"
              @click="revoke(row as AgentKey)"
            >
              撤销
            </el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="还没有密钥" :image-size="80" />
        </template>
      </el-table>
    </el-card>

    <CreateKeyDialog
      v-model="dialogVisible"
      :base-url="baseUrl"
      @created="load"
    />
  </div>
</template>
