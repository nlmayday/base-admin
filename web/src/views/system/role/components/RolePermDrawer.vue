<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { message } from "@/utils/message";
import {
  ErrorCode,
  hasErrorCode,
  type PermissionDependencyData
} from "@/utils/http/errors";
import { useUserStoreHook } from "@/store/modules/user";
import { refreshMenus } from "@/router/utils";
import {
  getPermissionTree,
  getRolePermissions,
  setRolePermissions,
  type PermNode,
  type Role
} from "@/api/iam/role";
import {
  buildRequiresMap,
  collectPermissions,
  flattenRows,
  groupState,
  toggleMany,
  togglePermission,
  type RequiresMap
} from "../utils/permDeps";

defineOptions({ name: "RolePermDrawer" });

const props = defineProps<{ role: Role | null }>();
const visible = defineModel<boolean>({ required: true });

const TYPE_LABEL: Record<PermNode["type"], string> = {
  dir: "目录",
  menu: "菜单",
  link: "外链"
};

const loading = ref(false);
const saving = ref(false);
const tree = ref<PermNode[]>([]);
const requiresMap = ref<RequiresMap>(new Map());
const checked = ref<ReadonlySet<string>>(new Set());
const missing = ref<ReadonlySet<string>>(new Set());
const unknownFromServer = ref<string[]>([]);

const knownCodes = computed(
  () => new Set(collectPermissions(tree.value).map(p => p.code))
);
/** 角色当前拥有、但权限树中不存在的权限码 */
const unknownChecked = computed(() =>
  [...checked.value].filter(c => !knownCodes.value.has(c)).sort()
);
const nameOf = computed(
  () =>
    new Map(collectPermissions(tree.value).map(p => [p.code, p.name] as const))
);

function resetErrors(): void {
  missing.value = new Set();
  unknownFromServer.value = [];
}

async function load(role: Role): Promise<void> {
  loading.value = true;
  resetErrors();
  try {
    const [permTree, current] = await Promise.all([
      getPermissionTree(),
      getRolePermissions(role.id)
    ]);
    tree.value = permTree ?? [];
    requiresMap.value = buildRequiresMap(tree.value);
    checked.value = new Set(current?.codes ?? []);
  } catch {
    visible.value = false;
  } finally {
    loading.value = false;
  }
}

watch(visible, open => {
  if (open && props.role) load(props.role);
});

function onToggle(code: string, on: boolean): void {
  checked.value = togglePermission(checked.value, code, on, requiresMap.value);
  if (missing.value.has(code) && on) {
    missing.value = new Set([...missing.value].filter(c => c !== code));
  }
}

const rows = computed(() => flattenRows(tree.value));

function onToggleGroup(codes: readonly string[]): void {
  const on = groupState(checked.value, codes) !== "all";
  checked.value = toggleMany(checked.value, codes, on, requiresMap.value);
}

function removeUnknown(code: string): void {
  checked.value = new Set([...checked.value].filter(c => c !== code));
  unknownFromServer.value = unknownFromServer.value.filter(c => c !== code);
}

function labelOf(code: string): string {
  const name = nameOf.value.get(code);
  return name ? `${name}（${code}）` : code;
}

async function save(): Promise<void> {
  if (!props.role) return;
  saving.value = true;
  resetErrors();
  try {
    const result = await setRolePermissions(
      props.role.id,
      [...checked.value].sort()
    );
    checked.value = new Set(result?.codes ?? checked.value);
    message("权限已保存", { type: "success" });
    visible.value = false;
    // 当前用户可能拥有该角色，重新拉取 /me 更新按钮权限
    useUserStoreHook()
      .fetchMe()
      .then(() => refreshMenus())
      .catch(err => console.warn("[role] refresh /me failed", err));
  } catch (err) {
    if (hasErrorCode(err, ErrorCode.PERMISSION_DEPENDENCY)) {
      const data = (err.data ?? {}) as Partial<PermissionDependencyData>;
      missing.value = new Set(data.missing ?? []);
      unknownFromServer.value = [...(data.unknown ?? [])];
      message(err.message || "权限依赖不满足", { type: "warning" });
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="`分配权限：${role?.name ?? ''}`"
    size="560px"
    destroy-on-close
  >
    <div v-loading="loading" class="flex h-full flex-col">
      <el-alert
        v-if="missing.size > 0"
        type="error"
        :closable="false"
        class="mb-3"
        title="缺少依赖的权限（已标红）"
      >
        <div v-for="code in missing" :key="code">{{ labelOf(code) }}</div>
      </el-alert>
      <el-alert
        v-if="unknownFromServer.length > 0 || unknownChecked.length > 0"
        type="warning"
        :closable="false"
        class="mb-3"
        title="未知的权限码（点击移除）"
      >
        <el-tag
          v-for="code in [
            ...new Set([...unknownFromServer, ...unknownChecked])
          ]"
          :key="code"
          closable
          type="warning"
          class="mr-1 mb-1"
          @close="removeUnknown(code)"
        >
          {{ code }}
        </el-tag>
      </el-alert>
      <p class="mb-2 text-xs text-[var(--el-text-color-secondary)]">
        勾选操作权限会自动勾选它依赖的权限；取消某权限会同时取消依赖它的权限。
      </p>
      <el-scrollbar class="flex-1">
        <div class="perm-table">
          <div
            v-for="row in rows"
            :key="row.node.menuCode || row.node.title"
            class="perm-row"
            :class="{ 'perm-row--group': row.node.permissions.length === 0 }"
          >
            <div
              class="perm-label"
              :style="{ paddingLeft: `${row.depth * 16}px` }"
            >
              <el-checkbox
                v-if="row.codes.length > 0"
                :model-value="groupState(checked, row.codes) === 'all'"
                :indeterminate="groupState(checked, row.codes) === 'some'"
                :data-perm-group="row.node.menuCode"
                @change="onToggleGroup(row.codes)"
              >
                <span class="font-medium">{{ row.node.title }}</span>
              </el-checkbox>
              <el-tag size="small" type="info" class="ml-1">
                {{ TYPE_LABEL[row.node.type] }}
              </el-tag>
            </div>
            <div v-if="row.node.permissions.length > 0" class="perm-items">
              <el-checkbox
                v-for="p in row.node.permissions"
                :key="p.code"
                :model-value="checked.has(p.code)"
                :class="{ 'perm-missing': missing.has(p.code) }"
                :data-perm-code="p.code"
                :title="p.code"
                @change="
                  (v: boolean | string | number) => onToggle(p.code, Boolean(v))
                "
              >
                {{ p.name }}
              </el-checkbox>
            </div>
          </div>
        </div>
      </el-scrollbar>
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-drawer>
</template>

<style scoped>
.perm-row {
  display: flex;
  align-items: flex-start;
  padding: 6px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.perm-row--group {
  background: var(--el-fill-color-light);
}

.perm-label {
  display: flex;
  flex: 0 0 180px;
  align-items: center;
  min-height: 32px;
  padding-right: 8px;
}

.perm-items {
  display: grid;
  flex: 1;
  grid-template-columns: repeat(auto-fill, minmax(104px, 1fr));
  column-gap: 8px;
}

.perm-missing :deep(.el-checkbox__label) {
  font-weight: 600;
  color: var(--el-color-danger);
}
</style>
