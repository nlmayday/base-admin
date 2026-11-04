<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { formatDateTime } from "@/utils/format";
import { hasPerm } from "@/utils/permission";
import {
  listSessions,
  revokeSession,
  revokeUserSessions,
  type OnlineSession
} from "@/api/security";
import { describeAgent, relativeTime } from "./agent";

defineOptions({ name: "AuthSessions" });

const keyword = ref("");
const canRevoke = computed(() => hasPerm("auth:session:revoke"));

const table = usePagedTable<OnlineSession>(q =>
  listSessions({ ...q, keyword: keyword.value.trim() || undefined })
);
const { loading, rows, pagination, reload, search } = table;

const userCount = computed(() => new Set(rows.value.map(r => r.userId)).size);

function kick(row: OnlineSession): void {
  confirmAction({
    confirm: `确定让「${row.nickname || row.username}」在 ${row.ip || "未知 IP"} 的会话下线吗？`,
    success: "已下线",
    action: () => revokeSession(row.id),
    reload
  });
}

function kickUser(row: OnlineSession): void {
  confirmAction({
    confirm: `确定让「${row.nickname || row.username}」的全部会话下线吗？该用户所有设备都需要重新登录。`,
    success: "该用户已全部下线",
    action: () => revokeUserSessions(row.userId),
    reload
  });
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input
            v-model="keyword"
            placeholder="用户名 / 昵称 / IP"
            clearable
            class="w-[240px]!"
            @keyup.enter="search"
            @clear="search"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="search">搜索</el-button>
          <el-button :loading="loading" @click="reload">刷新</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <span class="font-medium">在线会话</span>
          <el-tag type="success" effect="plain" round>
            {{ pagination.total }} 个会话
          </el-tag>
          <el-tag type="info" effect="plain" round>
            本页 {{ userCount }} 位用户
          </el-tag>
        </div>
        <span class="text-xs text-[var(--el-text-color-secondary)]">
          会话空闲超时或到达最长有效期后自动失效
        </span>
      </div>

      <el-table v-loading="loading" :data="rows" row-key="id">
        <el-table-column label="用户" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <el-avatar :size="28" class="shrink-0">
                {{ (row.nickname || row.username || "?").slice(0, 1) }}
              </el-avatar>
              <div class="leading-tight">
                <div class="font-medium">
                  {{ row.nickname || row.username }}
                  <el-tag
                    v-if="row.current"
                    size="small"
                    type="primary"
                    class="ml-1"
                  >
                    当前会话
                  </el-tag>
                </div>
                <div class="text-xs text-[var(--el-text-color-secondary)]">
                  {{ row.username }}
                </div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="IP" prop="ip" width="150" />
        <el-table-column label="终端" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tooltip :content="row.userAgent || '未知'" placement="top">
              <span>{{ describeAgent(row.userAgent) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="登录时间" width="215">
          <template #default="{ row }">
            {{ formatDateTime(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="最近活动" width="130">
          <template #default="{ row }">
            <el-tooltip
              :content="formatDateTime(row.lastSeenAt)"
              placement="top"
            >
              <span>{{ relativeTime(row.lastSeenAt) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="最长有效至" width="215">
          <template #default="{ row }">
            {{ formatDateTime(row.absoluteExpiresAt) }}
          </template>
        </el-table-column>
        <el-table-column
          v-if="canRevoke"
          label="操作"
          width="150"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              link
              type="warning"
              :disabled="row.current"
              @click="kick(row as OnlineSession)"
            >
              下线
            </el-button>
            <el-button
              link
              type="danger"
              :disabled="row.current"
              title="让该用户在所有设备上下线"
              @click="kickUser(row as OnlineSession)"
            >
              全部下线
            </el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无在线会话" :image-size="80" />
        </template>
      </el-table>

      <el-pagination
        class="mt-3 justify-end"
        :current-page="pagination.currentPage"
        :page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="table.onSizeChange"
        @current-change="table.onCurrentChange"
      />
    </el-card>
  </div>
</template>
