<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import ReIconSelect from "@/components/ReIconSelect/index.vue";
import type { FormInstance, FormRules } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { useFieldErrors } from "@/composables/useFieldErrors";
import { collectSubtreeIds, treeSelectProps } from "@/utils/tree-options";
import { createMenu, updateMenu, type MenuNode } from "@/api/iam/menu";

defineOptions({ name: "MenuFormDialog" });

// 模板属性里写 "//" 会让 vue-tsc（3.3）误判后续 v-model 的类型，放在脚本里
const URL_PLACEHOLDER = "https://";
const parentTreeProps = treeSelectProps("title", { disabled: "disabled" });

const props = defineProps<{
  menu: MenuNode | null;
  parentId: string | null;
  tree: MenuNode[];
}>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "saved"): void; (e: "reload"): void }>();

type MenuForm = {
  type: "dir" | "link";
  parentId: string | null;
  title: string;
  path: string;
  externalUrl: string;
  icon: string;
  sort: number;
  hidden: boolean;
};

const emptyForm = (): MenuForm => ({
  type: "dir",
  parentId: null,
  title: "",
  path: "",
  externalUrl: "",
  icon: "",
  sort: 0,
  hidden: false
});

const formRef = ref<FormInstance>();
const saving = ref(false);
const form = reactive<MenuForm>(emptyForm());
const { fieldErrors, clearFieldErrors, applyFieldErrors } = useFieldErrors();

const isEdit = computed(() => props.menu !== null);
/** 模块菜单只能修改 title / icon / sort / hidden */
const isModule = computed(() => props.menu?.source === "module");
const currentType = computed(() => props.menu?.type ?? form.type);

type ParentOption = {
  id: string;
  title: string;
  disabled: boolean;
  children: ParentOption[];
};

/** 上级只能是目录；编辑时排除自身及后代 */
const parentOptions = computed<ParentOption[]>(() => {
  const excluded = props.menu
    ? collectSubtreeIds(props.tree, props.menu.id)
    : new Set<string>();
  const build = (nodes: MenuNode[]): ParentOption[] =>
    (nodes ?? [])
      .filter(n => n.type === "dir")
      .map(n => ({
        id: n.id,
        title: n.title,
        disabled: excluded.has(n.id),
        children: build(n.children ?? [])
      }));
  return build(props.tree);
});

const rules = computed<FormRules>(() => ({
  title: [{ required: true, message: "请输入标题", trigger: "blur" }],
  path:
    !isModule.value && currentType.value === "dir"
      ? [
          { required: true, message: "请输入路由路径", trigger: "blur" },
          { pattern: /^\//, message: "路径必须以 / 开头", trigger: "blur" }
        ]
      : [],
  externalUrl:
    !isModule.value && currentType.value === "link"
      ? [
          { required: true, message: "请输入外链地址", trigger: "blur" },
          {
            pattern: /^https?:\/\//,
            message: "请输入 http(s):// 开头的地址",
            trigger: "blur"
          }
        ]
      : []
}));

watch(visible, open => {
  if (!open) return;
  clearFieldErrors();
  const m = props.menu;
  Object.assign(
    form,
    m
      ? {
          type: m.type === "link" ? "link" : "dir",
          parentId: m.parentId,
          title: m.title,
          path: m.path,
          externalUrl: m.externalUrl,
          icon: m.icon,
          sort: m.sort,
          hidden: m.hidden
        }
      : { ...emptyForm(), parentId: props.parentId }
  );
  formRef.value?.clearValidate();
});

function buildUpdatePayload(menu: MenuNode) {
  const base = {
    title: form.title,
    icon: form.icon,
    sort: form.sort,
    hidden: form.hidden,
    version: menu.version
  };
  if (menu.source === "module") return base;
  return {
    ...base,
    parentId: form.parentId || null,
    path: form.path,
    externalUrl: form.externalUrl
  };
}

async function submit(): Promise<void> {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  clearFieldErrors();
  saving.value = true;
  try {
    if (props.menu) {
      await updateMenu(props.menu.id, buildUpdatePayload(props.menu));
    } else {
      await createMenu({ ...form, parentId: form.parentId || null });
    }
    message(isEdit.value ? "菜单已更新" : "菜单已创建", { type: "success" });
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
    :title="isEdit ? '编辑菜单' : '新增菜单'"
    width="560px"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <el-alert
      v-if="isModule"
      type="info"
      :closable="false"
      class="mb-4"
      title="模块菜单由后端模块注册，只能修改标题、图标、排序和是否隐藏。"
    />
    <!-- 规则随类型变化；关闭 validate-on-rule-change，避免切换类型时立即报错 -->
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :validate-on-rule-change="false"
      label-width="90px"
    >
      <el-form-item v-if="!isEdit" label="类型" prop="type">
        <el-radio-group v-model="form.type">
          <el-radio value="dir">目录</el-radio>
          <el-radio value="link">外链</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item
        v-if="!isModule"
        label="上级目录"
        prop="parentId"
        :error="fieldErrors.parentId"
      >
        <el-tree-select
          v-model="form.parentId"
          :data="parentOptions"
          :props="parentTreeProps"
          node-key="id"
          check-strictly
          clearable
          default-expand-all
          placeholder="不选则为顶级"
          class="w-full"
        />
      </el-form-item>
      <el-form-item label="标题" prop="title" :error="fieldErrors.title">
        <el-input v-model="form.title" maxlength="64" />
      </el-form-item>
      <el-form-item
        v-if="!isModule"
        label="路由路径"
        prop="path"
        :error="fieldErrors.path"
      >
        <el-input
          v-model="form.path"
          :placeholder="currentType === 'dir' ? '/example' : '可选'"
        />
      </el-form-item>
      <el-form-item
        v-if="!isModule && currentType === 'link'"
        label="外链地址"
        prop="externalUrl"
        :error="fieldErrors.externalUrl"
      >
        <el-input v-model="form.externalUrl" :placeholder="URL_PLACEHOLDER" />
      </el-form-item>
      <el-form-item label="图标" prop="icon" :error="fieldErrors.icon">
        <ReIconSelect v-model="form.icon" />
      </el-form-item>
      <el-form-item label="排序" prop="sort" :error="fieldErrors.sort">
        <el-input-number v-model="form.sort" :min="0" :max="9999" />
      </el-form-item>
      <el-form-item label="隐藏" prop="hidden">
        <el-switch v-model="form.hidden" />
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
