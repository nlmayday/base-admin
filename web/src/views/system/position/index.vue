<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import {
  deletePosition,
  listPositions,
  type Position
} from "@/api/iam/position";
import ReStatusTag from "@/components/ReStatusTag/index.vue";
import PositionFormDialog from "./components/PositionFormDialog.vue";
import { hasPerm } from "@/utils/permission";
import { computed } from "vue";

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  ["iam:position:update", "iam:position:delete"].some(code => hasPerm(code))
);

defineOptions({ name: "IamPosition" });

const filter = reactive({ keyword: "" });
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<Position>(query => listPositions({ ...query, ...filter }));

const dialogVisible = ref(false);
const editing = ref<Position | null>(null);

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: Position): void {
  editing.value = row;
  dialogVisible.value = true;
}

function remove(row: Position): void {
  confirmAction({
    confirm: `确定删除岗位「${row.name}」吗？`,
    success: "已删除",
    action: () => deletePosition(row.id),
    reload
  });
}

function resetFilter(): void {
  filter.keyword = "";
  search();
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input
            v-model="filter.keyword"
            placeholder="岗位编码/名称"
            clearable
            class="w-[220px]!"
            @keyup.enter="search"
          />
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
        <span class="font-medium">岗位列表</span>
        <el-button
          v-perm="'iam:position:create'"
          type="primary"
          data-testid="position-create"
          @click="openCreate"
        >
          新增岗位
        </el-button>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <el-table-column prop="code" label="编码" min-width="140" />
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="sort" label="排序" width="80" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <ReStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="160" />
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="150"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-perm="'iam:position:update'"
              link
              type="primary"
              @click="openEdit(row as Position)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'iam:position:delete'"
              link
              type="danger"
              @click="remove(row as Position)"
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

    <PositionFormDialog
      v-model="dialogVisible"
      :position="editing"
      @saved="reload"
      @reload="reload"
    />
  </div>
</template>
