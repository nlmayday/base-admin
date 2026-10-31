<script setup lang="ts">
import { computed } from "vue";
import type { FileItem } from "@/api/file";
import { hasPerm } from "@/utils/permission";
import type { FileAction } from "../types";
import MoreIcon from "~icons/ri/more-2-fill";
import EditIcon from "~icons/ri/edit-line";
import MoveIcon from "~icons/ri/folder-transfer-line";
import DeleteIcon from "~icons/ri/delete-bin-line";

defineOptions({ name: "FileActions" });

defineProps<{ file: FileItem }>();
const emit = defineEmits<{ (e: "action", action: FileAction): void }>();

const canUpdate = computed(() => hasPerm("file:file:update"));
const canDelete = computed(() => hasPerm("file:file:delete"));

function onCommand(command: FileAction): void {
  emit("action", command);
}
</script>

<template>
  <el-dropdown
    v-if="canUpdate || canDelete"
    trigger="click"
    @command="onCommand"
  >
    <el-button link class="file-actions__trigger" title="更多" @click.stop>
      <MoreIcon />
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item v-if="canUpdate" command="rename">
          <EditIcon class="mr-2" />重命名
        </el-dropdown-item>
        <el-dropdown-item v-if="canUpdate" command="move">
          <MoveIcon class="mr-2" />移动到…
        </el-dropdown-item>
        <el-dropdown-item
          v-if="canDelete"
          command="delete"
          :divided="canUpdate"
          class="text-[var(--el-color-danger)]!"
        >
          <DeleteIcon class="mr-2" />删除
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>
