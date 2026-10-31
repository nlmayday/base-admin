<script setup lang="ts">
import { onMounted, ref } from "vue";
import { confirmAction } from "@/composables/useConfirmAction";
import { useRenderIcon } from "@/components/ReIcon/src/hooks";
import { computed } from "vue";
import {
  deleteMenu,
  getMenuTree,
  type MenuNode,
  type MenuType
} from "@/api/iam/menu";
import { refreshMenus } from "@/router/utils";
import MenuFormDialog from "./components/MenuFormDialog.vue";
import { hasPerm } from "@/utils/permission";

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  ["iam:menu:create", "iam:menu:update", "iam:menu:delete"].some(code =>
    hasPerm(code)
  )
);

defineOptions({ name: "IamMenu" });

type RowType = MenuType | "perm";

/** 表格行：菜单节点，或挂在菜单下的只读按钮权限 */
type MenuRow = Omit<MenuNode, "type" | "children"> & {
  type: RowType;
  children: MenuRow[];
};

const TYPE_META: Record<
  RowType,
  { label: string; tag: "primary" | "success" | "warning" | "info" }
> = {
  dir: { label: "目录", tag: "primary" },
  menu: { label: "菜单", tag: "success" },
  link: { label: "外链", tag: "warning" },
  perm: { label: "按钮", tag: "info" }
};

function toRows(nodes: readonly MenuNode[]): MenuRow[] {
  return nodes.map(node => ({
    ...node,
    children: [
      ...toRows(node.children ?? []),
      ...(node.permissions ?? []).map((p): MenuRow => ({
        ...node,
        id: `perm:${p.code}`,
        code: p.code,
        type: "perm",
        title: p.name,
        path: "",
        component: "",
        icon: "",
        permissionCode: p.code,
        externalUrl: "",
        permissions: [],
        children: []
      }))
    ]
  }));
}

const loading = ref(false);
const tree = ref<MenuNode[]>([]);
const dialogVisible = ref(false);
const editing = ref<MenuNode | null>(null);
const presetParentId = ref<string | null>(null);
const rows = computed(() => toRows(tree.value));

/** 默认只展开目录，菜单下的按钮行收起，点击展开查看 */
const expandedKeys = computed(() => {
  const keys: string[] = [];
  const visit = (nodes: readonly MenuNode[]): void => {
    for (const n of nodes) {
      if (n.type === "dir") keys.push(n.id);
      visit(n.children ?? []);
    }
  };
  visit(tree.value);
  return keys;
});

async function reload(): Promise<void> {
  loading.value = true;
  try {
    tree.value = (await getMenuTree()) ?? [];
  } catch {
    tree.value = [];
  } finally {
    loading.value = false;
  }
}

function openCreate(parent: MenuNode | null): void {
  editing.value = null;
  presetParentId.value = parent?.id ?? null;
  dialogVisible.value = true;
}

function openEdit(row: MenuNode): void {
  editing.value = row;
  presetParentId.value = null;
  dialogVisible.value = true;
}

function remove(row: MenuNode): void {
  confirmAction({
    confirm: `确定删除菜单「${row.title}」吗？`,
    success: "已删除",
    action: () => deleteMenu(row.id),
    reload: afterChange
  });
}

/** 菜单变更后同时刷新列表和侧边栏 */
async function afterChange(): Promise<void> {
  await reload();
  refreshMenus().catch(err =>
    console.warn("[menu] refresh sidebar failed", err)
  );
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="font-medium">菜单列表</span>
          <span class="text-xs text-[var(--el-text-color-secondary)]">
            模块菜单由后端注册，只能修改标题、图标、排序和隐藏；可新增自定义目录与外链。按钮为菜单下的操作权限，在角色管理中分配。
          </span>
        </div>
        <div>
          <el-button :loading="loading" @click="reload">刷新</el-button>
          <el-button
            v-perm="'iam:menu:create'"
            type="primary"
            data-testid="menu-create"
            @click="openCreate(null)"
          >
            新增
          </el-button>
        </div>
      </div>
      <el-table
        v-loading="loading"
        :data="rows"
        row-key="id"
        border
        :expand-row-keys="expandedKeys"
        :tree-props="{ children: 'children' }"
      >
        <el-table-column prop="title" label="标题" min-width="180" />
        <el-table-column label="图标" width="70" align="center">
          <template #default="{ row }">
            <component :is="useRenderIcon(row.icon)" v-if="row.icon" />
          </template>
        </el-table-column>
        <el-table-column label="类型" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="TYPE_META[row.type as RowType]?.tag" size="small">
              {{ TYPE_META[row.type as RowType]?.label ?? row.type }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="路径 / 地址" min-width="200">
          <template #default="{ row }">
            {{ row.type === "link" ? row.externalUrl : row.path }}
          </template>
        </el-table-column>
        <el-table-column prop="component" label="组件" min-width="160" />
        <el-table-column prop="permissionCode" label="权限码" min-width="150" />
        <el-table-column label="排序" width="70" align="center">
          <template #default="{ row }">
            {{ row.type === "perm" ? "" : row.sort }}
          </template>
        </el-table-column>
        <el-table-column label="隐藏" width="70" align="center">
          <template #default="{ row }">
            {{ row.type === "perm" ? "" : row.hidden ? "是" : "否" }}
          </template>
        </el-table-column>
        <el-table-column label="来源" width="80" align="center">
          <template #default="{ row }">
            <el-tag
              v-if="row.type !== 'perm'"
              :type="row.source === 'module' ? 'info' : 'success'"
              size="small"
            >
              {{ row.source === "module" ? "模块" : "自定义" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="200"
          fixed="right"
        >
          <template #default="{ row }">
            <span
              v-if="row.type === 'perm'"
              class="text-xs text-[var(--el-text-color-secondary)]"
            >
              在角色管理中分配
            </span>
            <template v-else>
              <el-button
                v-if="row.type === 'dir'"
                v-perm="'iam:menu:create'"
                link
                type="primary"
                @click="openCreate(row as MenuNode)"
              >
                新增下级
              </el-button>
              <el-button
                v-perm="'iam:menu:update'"
                link
                type="primary"
                @click="openEdit(row as MenuNode)"
              >
                编辑
              </el-button>
              <el-button
                v-perm="'iam:menu:delete'"
                link
                type="danger"
                :disabled="row.source === 'module'"
                @click="remove(row as MenuNode)"
              >
                删除
              </el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <MenuFormDialog
      v-model="dialogVisible"
      :menu="editing"
      :parent-id="presetParentId"
      :tree="tree"
      @saved="afterChange"
      @reload="reload"
    />
  </div>
</template>
