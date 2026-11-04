<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useUserStoreHook } from "@/store/modules/user";
import { ElMessageBox } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { formatDateTime } from "@/utils/format";
import {
  STATUS_OPTIONS,
  STATUS_DISABLED,
  STATUS_ENABLED,
  type Status
} from "@/api/types";
import { getDeptTree, type Dept } from "@/api/iam/dept";
import {
  deleteUser,
  exportUsers,
  listUsers,
  resetUserPassword,
  updateUserStatus,
  type User
} from "@/api/iam/user";
import ReStatusTag from "@/components/ReStatusTag/index.vue";
import { PASSWORD_MAX, PASSWORD_MIN } from "@/views/login/utils/rule";
import DeptTreePanel from "./components/DeptTreePanel.vue";
import UserFormDialog from "./components/UserFormDialog.vue";
import UserImportDialog from "./components/UserImportDialog.vue";
import { hasPerm } from "@/utils/permission";

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  ["iam:user:update", "iam:user:reset-password", "iam:user:delete"].some(code =>
    hasPerm(code)
  )
);

/** 当前登录用户的 id：不能停用或删除自己 */
const selfId = computed(() => useUserStoreHook().me?.user.id ?? "");

defineOptions({ name: "IamUser" });

const filter = reactive<{
  keyword: string;
  deptId: string | null;
  status: Status | null;
}>({ keyword: "", deptId: null, status: null });

const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<User>(query => listUsers({ ...query, ...filter }));

const deptTree = ref<Dept[]>([]);
const deptLoading = ref(false);
const dialogVisible = ref(false);
const editing = ref<User | null>(null);
const exporting = ref(false);
const importVisible = ref(false);

async function loadDeptTree(): Promise<void> {
  deptLoading.value = true;
  try {
    deptTree.value = (await getDeptTree()) ?? [];
  } catch {
    deptTree.value = [];
  } finally {
    deptLoading.value = false;
  }
}

function onDeptSelect(deptId: string | null): void {
  filter.deptId = deptId;
  search();
}

function resetFilter(): void {
  filter.keyword = "";
  filter.status = null;
  search();
}

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: User): void {
  editing.value = row;
  dialogVisible.value = true;
}

function toggleStatus(row: User): void {
  const next: Status =
    row.status === STATUS_ENABLED ? STATUS_DISABLED : STATUS_ENABLED;
  const verb = next === STATUS_ENABLED ? "启用" : "停用";
  confirmAction({
    confirm: `确定${verb}用户「${row.username}」吗？${next === STATUS_DISABLED ? "停用后该用户的会话会立即失效。" : ""}`,
    success: `已${verb}`,
    action: () => updateUserStatus(row.id, next, row.version),
    reload
  });
}

function remove(row: User): void {
  confirmAction({
    confirm: `确定删除用户「${row.username}」吗？`,
    success: "已删除",
    action: () => deleteUser(row.id),
    reload
  });
}

async function resetPassword(row: User): Promise<void> {
  let password: string;
  try {
    const result = await ElMessageBox.prompt(
      `为用户「${row.username}」设置新密码，该用户的所有会话将被注销。`,
      "重置密码",
      {
        inputType: "password",
        inputPlaceholder: `${PASSWORD_MIN}–${PASSWORD_MAX} 位`,
        inputValidator: (v: string) =>
          (v?.length ?? 0) >= PASSWORD_MIN && v.length <= PASSWORD_MAX
            ? true
            : `密码长度为 ${PASSWORD_MIN}–${PASSWORD_MAX} 位`,
        confirmButtonText: "确定",
        cancelButtonText: "取消"
      }
    );
    password = result.value;
  } catch {
    return; // 取消
  }
  try {
    await resetUserPassword(row.id, password);
    message("密码已重置", { type: "success" });
  } catch (err) {
    if (needsReload(err)) reload();
  }
}

async function onExport(): Promise<void> {
  exporting.value = true;
  try {
    await exportUsers({ ...filter });
  } catch {
    // 错误提示已由 http 拦截器处理
  } finally {
    exporting.value = false;
  }
}

onMounted(() => {
  loadDeptTree();
  reload();
});
</script>

<template>
  <div class="main split-page flex gap-3">
    <div class="w-[240px] shrink-0">
      <DeptTreePanel
        :data="deptTree"
        :loading="deptLoading"
        @select="onDeptSelect"
      />
    </div>
    <div class="min-w-0 flex-1">
      <el-card shadow="never" class="mb-3" body-class="pb-0!">
        <el-form :inline="true" :model="filter" @submit.prevent="search">
          <el-form-item label="关键字">
            <el-input
              v-model="filter.keyword"
              placeholder="用户名/昵称/手机/邮箱"
              clearable
              class="w-[220px]!"
              @keyup.enter="search"
            />
          </el-form-item>
          <el-form-item label="状态">
            <el-select
              v-model="filter.status"
              placeholder="全部"
              clearable
              class="w-[120px]!"
            >
              <el-option
                v-for="o in STATUS_OPTIONS"
                :key="o.value"
                :label="o.label"
                :value="o.value"
              />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="search">
              搜索
            </el-button>
            <el-button @click="resetFilter">重置</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <el-card shadow="never">
        <div class="mb-3 flex justify-between">
          <span class="font-medium">用户列表</span>
          <div>
            <el-button
              v-perm="'iam:user:create'"
              type="primary"
              data-testid="user-create"
              @click="openCreate"
            >
              新增用户
            </el-button>
            <el-button
              v-perm="'iam:user:export'"
              :loading="exporting"
              data-testid="user-export"
              @click="onExport"
            >
              导出 CSV
            </el-button>
            <el-button
              v-perm="'iam:user:import'"
              data-testid="user-import"
              @click="importVisible = true"
            >
              导入
            </el-button>
          </div>
        </div>
        <el-table v-loading="loading" :data="rows" row-key="id" border>
          <el-table-column prop="username" label="用户名" min-width="120" />
          <el-table-column prop="nickname" label="昵称" min-width="120" />
          <el-table-column prop="deptName" label="部门" min-width="120">
            <template #default="{ row }">{{ row.deptName || "-" }}</template>
          </el-table-column>
          <el-table-column prop="phone" label="手机号" min-width="120" />
          <el-table-column prop="email" label="邮箱" min-width="160" />
          <el-table-column label="状态" width="80" align="center">
            <template #default="{ row }">
              <ReStatusTag :status="row.status" />
            </template>
          </el-table-column>
          <el-table-column label="最后登录" width="215">
            <template #default="{ row }">
              {{ formatDateTime(row.lastLoginAt) }}
            </template>
          </el-table-column>
          <el-table-column
            v-if="showRowActions"
            label="操作"
            width="260"
            fixed="right"
          >
            <template #default="{ row }">
              <el-button
                v-perm="'iam:user:update'"
                link
                type="primary"
                @click="openEdit(row as User)"
              >
                编辑
              </el-button>
              <el-button
                v-perm="'iam:user:update'"
                link
                :type="row.status === 1 ? 'warning' : 'success'"
                :disabled="row.id === selfId"
                @click="toggleStatus(row as User)"
              >
                {{ row.status === 1 ? "停用" : "启用" }}
              </el-button>
              <el-button
                v-perm="'iam:user:reset-password'"
                link
                type="primary"
                @click="resetPassword(row as User)"
              >
                重置密码
              </el-button>
              <el-button
                v-perm="'iam:user:delete'"
                link
                type="danger"
                :disabled="row.id === selfId"
                @click="remove(row as User)"
              >
                删除
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination
          class="mt-3 justify-end"
          :current-page="pagination.currentPage"
          :page-size="pagination.pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="pagination.total"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @size-change="onSizeChange"
          @current-change="onCurrentChange"
        />
      </el-card>
    </div>

    <UserImportDialog v-model="importVisible" @imported="reload" />
    <UserFormDialog
      v-model="dialogVisible"
      :user="editing"
      :dept-tree="deptTree"
      @saved="reload"
      @reload="reload"
    />
  </div>
</template>
