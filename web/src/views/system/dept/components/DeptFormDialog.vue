<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import {
  collectSubtreeIds,
  markDisabled,
  treeSelectProps
} from "@/utils/tree-options";
import { STATUS_ENABLED, STATUS_OPTIONS, type Status } from "@/api/types";
import { createDept, updateDept, type Dept } from "@/api/iam/dept";
import { getUserOptions, type UserOption } from "@/api/iam/user";

defineOptions({ name: "DeptFormDialog" });

const parentTreeProps = treeSelectProps("name", { disabled: "disabled" });

const props = defineProps<{
  dept: Dept | null;
  /** 新增下级时预设的上级部门 */
  parentId: string | null;
  tree: Dept[];
}>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type DeptForm = {
  parentId: string | null;
  name: string;
  leaderUserId: string | null;
  sort: number;
  status: Status;
};

const emptyForm = (): DeptForm => ({
  parentId: null,
  name: "",
  leaderUserId: null,
  sort: 0,
  status: STATUS_ENABLED
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<DeptForm>(emptyForm());
const leaderOptions = ref<UserOption[]>([]);
const leaderLoading = ref(false);
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();
const isEdit = computed(() => props.dept !== null);

/** 编辑时不能把部门移动到自己或后代下面 */
const parentTree = computed(() =>
  props.dept
    ? markDisabled(props.tree, collectSubtreeIds(props.tree, props.dept.id))
    : props.tree
);

const rules: FormRules = {
  name: [{ required: true, message: "请输入部门名称", trigger: "blur" }]
};

async function searchLeaders(keyword: string): Promise<void> {
  leaderLoading.value = true;
  try {
    leaderOptions.value = (await getUserOptions(keyword || undefined)) ?? [];
  } catch {
    leaderOptions.value = [];
  } finally {
    leaderLoading.value = false;
  }
}

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const d = props.dept;
  Object.assign(
    form,
    d
      ? {
          parentId: d.parentId,
          name: d.name,
          leaderUserId: d.leaderUserId,
          sort: d.sort,
          status: d.status
        }
      : { ...emptyForm(), parentId: props.parentId }
  );
  formRef.value?.clearValidate();
  searchLeaders("");
});

function leaderLabel(o: UserOption): string {
  return o.nickname ? `${o.nickname}（${o.username}）` : o.username;
}

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  const payload = {
    ...form,
    parentId: form.parentId || null,
    leaderUserId: form.leaderUserId || null
  };
  try {
    if (props.dept) {
      await updateDept(props.dept.id, {
        ...payload,
        version: props.dept.version
      });
    } else {
      await createDept(payload);
    }
    message(isEdit.value ? "部门已更新" : "部门已创建", { type: "success" });
    visible.value = false;
    emit("saved");
  } catch (err) {
    applyFieldErrors(err);
    if (needsReload(err)) {
      visible.value = false;
      emit("reload");
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑部门' : '新增部门'"
    width="520px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="90px"
    >
      <el-form-item
        label="上级部门"
        prop="parentId"
        :error="fieldErrors.parentId"
      >
        <el-tree-select
          v-model="form.parentId"
          :data="parentTree"
          :props="parentTreeProps"
          node-key="id"
          check-strictly
          clearable
          filterable
          default-expand-all
          placeholder="不选则为顶级部门"
          class="w-full"
        />
      </el-form-item>
      <el-form-item label="部门名称" prop="name" :error="fieldErrors.name">
        <el-input v-model="form.name" maxlength="64" />
      </el-form-item>
      <el-form-item
        label="负责人"
        prop="leaderUserId"
        :error="fieldErrors.leaderUserId"
      >
        <el-select
          v-model="form.leaderUserId"
          filterable
          remote
          clearable
          :remote-method="searchLeaders"
          :loading="leaderLoading"
          placeholder="搜索用户"
          class="w-full"
        >
          <el-option
            v-for="o in leaderOptions"
            :key="o.id"
            :label="leaderLabel(o)"
            :value="o.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="排序" prop="sort" :error="fieldErrors.sort">
        <el-input-number v-model="form.sort" :min="0" :max="9999" />
      </el-form-item>
      <el-form-item label="状态" prop="status">
        <el-radio-group v-model="form.status">
          <el-radio v-for="o in STATUS_OPTIONS" :key="o.value" :value="o.value">
            {{ o.label }}
          </el-radio>
        </el-radio-group>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">
        保存
      </el-button>
    </template>
  </el-dialog>
</template>
